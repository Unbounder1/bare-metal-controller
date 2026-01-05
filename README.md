# bare-metal-controller

A Kubernetes controller and external cloud provider for autoscaling bare metal clusters.

---

## Overview

The bare-metal-controller enables automatic scaling of bare metal Kubernetes clusters by integrating with the [Kubernetes Cluster Autoscaler](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler) via the external gRPC cloud provider interface.

Unlike cloud environments where virtual machines can be created on-demand, bare metal scaling manages existing physical servers by controlling their power state:

- **Scale Up:** Power on an existing physical server using Wake-on-LAN, wait for it to boot and join the cluster
- **Scale Down:** Power off a server via SSH shutdown command

This controller exposes a gRPC server that implements the Cluster Autoscaler's cloud provider interface, allowing the autoscaler to manage bare metal nodes as if they were cloud instances.

---

## Architecture

### Components

**Server Controller:** Watches Server custom resources and reconciles the desired power state with the actual state. Executes power commands (WoL for power-on, SSH for power-off).

**gRPC Cloud Provider Server:** Implements the Kubernetes Cluster Autoscaler's external gRPC cloud provider interface. Translates autoscaler requests into Server resource operations.

### How It Works

```
┌─────────────────────┐     gRPC      ┌──────────────────────┐
│  Cluster Autoscaler │◄─────────────►│  gRPC Provider Server │
└─────────────────────┘               └──────────────────────┘
                                                 │
                                                 │ Kubernetes API
                                                 ▼
                                      ┌──────────────────────┐
                                      │   Server Resources   │
                                      └──────────────────────┘
                                                 │
                                                 │ Reconcile
                                                 ▼
                                      ┌──────────────────────┐
                                      │  Server Controller   │
                                      └──────────────────────┘
                                                 │
                                    ┌────────────┴────────────┐
                                    ▼                         ▼
                              Wake-on-LAN                SSH Shutdown
                              (Power On)                 (Power Off)
```

1. The Cluster Autoscaler determines scaling needs based on pending pods or resource utilization
2. It calls the gRPC cloud provider to increase or decrease node group size
3. The gRPC server updates Server resources' power state
4. The Server Controller reconciles by sending WoL packets or SSH shutdown commands
5. Servers boot up, join the cluster, and become available for scheduling

---

## Custom Resource Definition

### Server

The Server resource represents a single physical bare-metal machine. It is cluster-scoped.

```yaml
apiVersion: bare-metal.io/v1
kind: Server
metadata:
  name: worker-01
spec:
  powerState: "on"      # Desired state: "on" or "off"
  type: "wol"           # Control type: "wol" or "ipmi"
  control:
    wol:
      address: "192.168.1.101"
      macAddress: "00:11:22:33:44:55"
      broadcastAddress: "192.168.1.255"  # Optional
      port: 9                             # Default: 9
      sshSecretRef:
        name: server-ssh-credentials
        namespace: bare-metal-system
status:
  status: "active"      # Current status
  message: ""           # Optional status message
  failingSince: null    # Timestamp if failing
  failureCount: 0       # Number of consecutive failures
```

### Spec Fields

| Field | Type | Description |
|-------|------|-------------|
| `powerState` | `on` \| `off` | Desired power state of the server |
| `type` | `wol` \| `ipmi` | Power management control type |
| `control.wol.address` | string | IP address of the server |
| `control.wol.macAddress` | string | MAC address for Wake-on-LAN |
| `control.wol.broadcastAddress` | string | Broadcast address for WoL (optional) |
| `control.wol.port` | int | WoL port (default: 9) |
| `control.wol.user` | string | SSH username (optional, can use secret instead) |
| `control.wol.sshSecretRef` | object | Reference to Secret with SSH credentials |
| `control.ipmi.address` | string | IPMI interface address |
| `control.ipmi.username` | string | IPMI username |
| `control.ipmi.password` | string | IPMI password |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Current status: `pending`, `active`, `offline`, `draining`, `failed` |
| `message` | string | Human-readable status message |
| `failingSince` | timestamp | When the server started failing |
| `failureCount` | int | Number of consecutive failures |

---

## Power Management

### Wake-on-LAN (WoL)

Power-on uses Wake-on-LAN magic packets sent directly from the controller.

**Requirements:**
- WoL enabled in server BIOS/UEFI
- Server NIC supports WoL
- Controller on the same Layer 2 network as servers

The controller sends a magic packet containing the server's MAC address. The NIC receives this and triggers the boot sequence.

### SSH Shutdown

Power-off uses SSH to connect and execute a shutdown command.

**Requirements:**
- SSH service running on the server
- Valid credentials in the referenced Secret
- Network connectivity from controller to server

**SSH Secret Format:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: server-ssh-credentials
  namespace: bare-metal-system
type: Opaque
stringData:
  username: root
  ssh-privatekey: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAA...
    ...
    -----END OPENSSH PRIVATE KEY-----
```

| Field | Description |
|-------|-------------|
| `username` | SSH username for connecting to the server |
| `ssh-privatekey` | Private key in OpenSSH format |

### IPMI (Alternative)

For servers with IPMI/BMC interfaces, power management can use IPMI commands instead of WoL/SSH.

---

## gRPC Cloud Provider Interface

The controller implements the Kubernetes Cluster Autoscaler's external gRPC cloud provider interface.

### Supported Operations

| Method | Description |
|--------|-------------|
| `NodeGroups` | Returns available node groups (currently single "bare-metal-pool") |
| `NodeGroupNodes` | Lists all servers in a node group |
| `NodeGroupTargetSize` | Returns count of servers with `powerState: on` |
| `NodeGroupIncreaseSize` | Powers on additional servers |
| `NodeGroupDeleteNodes` | Powers off specified servers |
| `NodeGroupDecreaseTargetSize` | Powers off servers to reduce size |
| `NodeGroupForNode` | Returns the node group for a given node |
| `Refresh` | Refreshes cached state (no-op, queries API directly) |
| `Cleanup` | Cleanup on shutdown (no-op) |

### Node Group

Currently, all Server resources belong to a single node group: `bare-metal-pool`. The node group's maximum size equals the total number of Server resources.

---

## Installation

### Prerequisites

- Kubernetes cluster (v1.24+)
- Cluster Autoscaler configured for external gRPC provider
- Servers with Wake-on-LAN enabled (or IPMI)
- SSH access to servers for shutdown

### Deploy the Controller

```bash
# Clone the repository
git clone https://github.com/Unbounder1/bare-metal-controller.git
cd bare-metal-controller

# Install CRDs
make install

# Deploy the controller
make deploy IMG=<your-registry>/bare-metal-controller:latest
```

### Configure Cluster Autoscaler

Configure the Cluster Autoscaler to use the external gRPC provider:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cluster-autoscaler
spec:
  template:
    spec:
      containers:
      - name: cluster-autoscaler
        command:
        - ./cluster-autoscaler
        - --cloud-provider=externalgrpc
        - --cloud-config=/config/cloud-config
        volumeMounts:
        - name: cloud-config
          mountPath: /config
      volumes:
      - name: cloud-config
        configMap:
          name: autoscaler-cloud-config
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: autoscaler-cloud-config
data:
  cloud-config: |
    address: bare-metal-controller-grpc.bare-metal-system.svc:8086
```

---

## Usage

### Create Server Resources

Define your bare metal servers:

```yaml
apiVersion: bare-metal.io/v1
kind: Server
metadata:
  name: worker-01
spec:
  powerState: "off"
  type: "wol"
  control:
    wol:
      address: "192.168.1.101"
      macAddress: "00:11:22:33:44:55"
      sshSecretRef:
        name: server-ssh-credentials
        namespace: bare-metal-system
---
apiVersion: bare-metal.io/v1
kind: Server
metadata:
  name: worker-02
spec:
  powerState: "off"
  type: "wol"
  control:
    wol:
      address: "192.168.1.102"
      macAddress: "00:11:22:33:44:66"
      sshSecretRef:
        name: server-ssh-credentials
        namespace: bare-metal-system
```

### Manual Power Control

You can manually control server power state:

```bash
# Power on a server
kubectl patch server worker-01 --type=merge -p '{"spec":{"powerState":"on"}}'

# Power off a server
kubectl patch server worker-01 --type=merge -p '{"spec":{"powerState":"off"}}'

# Check server status
kubectl get servers
```

### Automatic Scaling

Once configured, the Cluster Autoscaler will automatically:

1. **Scale Up:** When pods are pending due to insufficient resources, power on additional servers
2. **Scale Down:** When nodes are underutilized, power off servers to save resources

---

## Configuration

### Controller Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--grpc-address` | `:8086` | gRPC server listen address |
| `--grpc-cert` | | TLS certificate file (optional) |
| `--grpc-key` | | TLS key file (optional) |
| `--grpc-ca` | | CA certificate file (optional) |
| `--metrics-bind-address` | `:8080` | Metrics endpoint address |
| `--health-probe-bind-address` | `:8081` | Health probe address |
| `--leader-elect` | `false` | Enable leader election |

### TLS Configuration

For secure gRPC communication:

```bash
./manager \
  --grpc-address=:8086 \
  --grpc-cert=/certs/server.crt \
  --grpc-key=/certs/server.key \
  --grpc-ca=/certs/ca.crt
```

---

## Status States

| Status | Description |
|--------|-------------|
| `pending` | Power state change initiated, waiting for result |
| `active` | Server is powered on and operational |
| `offline` | Server is powered off |
| `draining` | Server is being drained before shutdown |
| `failed` | Power operation failed |

---

## Troubleshooting

### Server Won't Power On

1. Verify WoL is enabled in BIOS/UEFI
2. Check MAC address is correct
3. Ensure controller is on same Layer 2 network
4. Check server status: `kubectl describe server <name>`

### Server Won't Power Off

1. Verify SSH credentials are correct
2. Check SSH service is running on server
3. Verify network connectivity
4. Check Secret exists and has correct data

### Autoscaler Not Scaling

1. Verify gRPC server is running: `kubectl logs -n bare-metal-system deployment/bare-metal-controller`
2. Check Cluster Autoscaler logs for gRPC errors
3. Verify cloud-config address is correct
4. Check Server resources exist: `kubectl get servers`

---

## Development

### Build

```bash
# Build the controller
make build

# Run tests
make test

# Run locally
make run
```

### Docker

```bash
# Build image
make docker-build IMG=bare-metal-controller:dev

# Push image
make docker-push IMG=<registry>/bare-metal-controller:dev
```

---

## Roadmap

- [ ] Multiple node groups with label selectors
- [ ] Health checks for server status verification
- [ ] Metrics collection from servers
- [ ] Graceful node drain before power-off
- [ ] IPMI power management support
- [ ] Multi-LAN support via relay agents

---

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0.