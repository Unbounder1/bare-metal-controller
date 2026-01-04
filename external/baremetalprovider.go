package protos

import (
	"context"
	"fmt"

	baremetalcontrollerv1 "github.com/Unbounder1/bare-metal-controller/api/v1"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BareMetalProviderServer struct {
	UnimplementedCloudProviderServer
	Client client.Client
}

const defaultNodeGroupID = "bare-metal-pool"

// NodeGroups returns all node groups configured for this cloud provider.
func (s *BareMetalProviderServer) NodeGroups(ctx context.Context, req *NodeGroupsRequest) (*NodeGroupsResponse, error) {
	var servers baremetalcontrollerv1.ServerList

	if err := s.Client.List(ctx, &servers); err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	// Current functionality: only support a single node group
	nodeGroups := []*NodeGroup{
		{
			Id:      defaultNodeGroupID,
			MinSize: 0,
			MaxSize: int32(len(servers.Items)),
		},
	}

	return &NodeGroupsResponse{
		NodeGroups: nodeGroups,
	}, nil
}

// NodeGroupIncreaseSize increases the size of a node group by provisioning
// offline servers.
func (s *BareMetalProviderServer) NodeGroupIncreaseSize(ctx context.Context, req *NodeGroupIncreaseSizeRequest) (*NodeGroupIncreaseSizeResponse, error) {
	nodeGroupID := req.GetId()

	if nodeGroupID != defaultNodeGroupID {
		return nil, fmt.Errorf("unknown node group: %s", nodeGroupID)
	}

	delta := int(req.GetDelta())
	if delta <= 0 {
		return &NodeGroupIncreaseSizeResponse{}, nil
	}

	var servers baremetalcontrollerv1.ServerList
	if err := s.Client.List(ctx, &servers); err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	provisioned := 0
	for i := range servers.Items {
		if provisioned >= delta {
			break
		}

		server := &servers.Items[i]
		if server.Spec.PowerState == baremetalcontrollerv1.PowerStateOff {
			server.Spec.PowerState = baremetalcontrollerv1.PowerStateOn
			if err := s.Client.Update(ctx, server); err != nil {
				return nil, fmt.Errorf("failed to power on server %s: %w", server.Name, err)
			}
			provisioned++
		}
	}

	if provisioned < delta {
		return nil, fmt.Errorf("could not provision enough servers: requested %d, provisioned %d", delta, provisioned)
	}

	return &NodeGroupIncreaseSizeResponse{}, nil
}

// NodeGroupDeleteNodes deletes nodes from a node group by powering off
// the corresponding servers.
func (s *BareMetalProviderServer) NodeGroupDeleteNodes(ctx context.Context, req *NodeGroupDeleteNodesRequest) (*NodeGroupDeleteNodesResponse, error) {
	nodeGroupID := req.GetId()

	if nodeGroupID != defaultNodeGroupID {
		return nil, fmt.Errorf("unknown node group: %s", nodeGroupID)
	}

	nodes := req.GetNodes()

	for _, node := range nodes {
		var server baremetalcontrollerv1.Server
		if err := s.Client.Get(ctx, client.ObjectKey{Name: node.Name}, &server); err != nil {
			return nil, fmt.Errorf("failed to get server %s: %w", node.Name, err)
		}

		server.Spec.PowerState = baremetalcontrollerv1.PowerStateOff
		if err := s.Client.Update(ctx, &server); err != nil {
			return nil, fmt.Errorf("failed to power off server %s: %w", server.Name, err)
		}
	}

	return &NodeGroupDeleteNodesResponse{}, nil
}

// NodeGroupForNode returns the node group that a given node belongs to.
func (s *BareMetalProviderServer) NodeGroupForNode(ctx context.Context, req *NodeGroupForNodeRequest) (*NodeGroupForNodeResponse, error) {
	node := req.GetNode()
	if node == nil {
		return nil, fmt.Errorf("node is required")
	}

	// Check if a server with this name exists
	var server baremetalcontrollerv1.Server
	if err := s.Client.Get(ctx, client.ObjectKey{Name: node.Name}, &server); err != nil {
		// Node not found in our inventory, return empty response
		return &NodeGroupForNodeResponse{}, nil
	}

	// All servers belong to the default node group
	return &NodeGroupForNodeResponse{
		NodeGroup: &NodeGroup{
			Id:      defaultNodeGroupID,
			MinSize: 0,
			MaxSize: s.getMaxSize(ctx),
		},
	}, nil
}

// NodeGroupTargetSize returns the current target size of the node group.
// Target size is the number of nodes that should be running.
func (s *BareMetalProviderServer) NodeGroupTargetSize(ctx context.Context, req *NodeGroupTargetSizeRequest) (*NodeGroupTargetSizeResponse, error) {
	nodeGroupID := req.GetId()

	if nodeGroupID != defaultNodeGroupID {
		return nil, fmt.Errorf("unknown node group: %s", nodeGroupID)
	}

	var servers baremetalcontrollerv1.ServerList
	if err := s.Client.List(ctx, &servers); err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	// Count servers that are powered on (target state)
	targetSize := int32(0)
	for _, server := range servers.Items {
		if server.Spec.PowerState == baremetalcontrollerv1.PowerStateOn {
			targetSize++
		}
	}

	return &NodeGroupTargetSizeResponse{
		TargetSize: targetSize,
	}, nil
}

// NodeGroupDecreaseTargetSize decreases the target size of the node group.
// This doesn't delete nodes but reduces the expected size.
func (s *BareMetalProviderServer) NodeGroupDecreaseTargetSize(ctx context.Context, req *NodeGroupDecreaseTargetSizeRequest) (*NodeGroupDecreaseTargetSizeResponse, error) {
	nodeGroupID := req.GetId()

	if nodeGroupID != defaultNodeGroupID {
		return nil, fmt.Errorf("unknown node group: %s", nodeGroupID)
	}

	delta := int(req.GetDelta())
	if delta <= 0 {
		return &NodeGroupDecreaseTargetSizeResponse{}, nil
	}

	var servers baremetalcontrollerv1.ServerList
	if err := s.Client.List(ctx, &servers); err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	// Power off 'delta' number of servers that are currently on
	powered_off := 0
	for i := range servers.Items {
		if powered_off >= delta {
			break
		}

		server := &servers.Items[i]
		if server.Spec.PowerState == baremetalcontrollerv1.PowerStateOn {
			server.Spec.PowerState = baremetalcontrollerv1.PowerStateOff
			if err := s.Client.Update(ctx, server); err != nil {
				return nil, fmt.Errorf("failed to power off server %s: %w", server.Name, err)
			}
			powered_off++
		}
	}

	return &NodeGroupDecreaseTargetSizeResponse{}, nil
}

// NodeGroupNodes returns a list of all nodes that belong to a node group.
func (s *BareMetalProviderServer) NodeGroupNodes(ctx context.Context, req *NodeGroupNodesRequest) (*NodeGroupNodesResponse, error) {
	nodeGroupID := req.GetId()

	if nodeGroupID != defaultNodeGroupID {
		return nil, fmt.Errorf("unknown node group: %s", nodeGroupID)
	}

	var servers baremetalcontrollerv1.ServerList
	if err := s.Client.List(ctx, &servers); err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	instances := make([]*Instance, 0, len(servers.Items))
	for _, server := range servers.Items {
		status := &InstanceStatus{
			InstanceState: s.mapPowerStateToInstanceState(server.Spec.PowerState),
		}

		instances = append(instances, &Instance{
			Id:     server.Name,
			Status: status,
		})
	}

	return &NodeGroupNodesResponse{
		Instances: instances,
	}, nil
}

// GPULabel returns the label key used to identify GPU nodes.
func (s *BareMetalProviderServer) GPULabel(ctx context.Context, req *GPULabelRequest) (*GPULabelResponse, error) {
	// Standard Kubernetes GPU label
	return &GPULabelResponse{
		Label: "nvidia.com/gpu",
	}, nil
}

// GetAvailableGPUTypes returns a map of available GPU types and their counts.
func (s *BareMetalProviderServer) GetAvailableGPUTypes(ctx context.Context, req *GetAvailableGPUTypesRequest) (*GetAvailableGPUTypesResponse, error) {
	var servers baremetalcontrollerv1.ServerList
	if err := s.Client.List(ctx, &servers); err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	gpuCounts := make(map[string]int64)

	for _, server := range servers.Items {
		// Check if server has GPU labels/annotations
		if gpuType, ok := server.Labels["gpu-type"]; ok {
			gpuCounts[gpuType]++
		}
	}

	// Convert to map[string]*anypb.Any
	gpuTypes := make(map[string]*anypb.Any)
	for gpuType, count := range gpuCounts {
		anyVal, err := anypb.New(wrapperspb.Int64(count))
		if err != nil {
			return nil, fmt.Errorf("failed to create Any value: %w", err)
		}
		gpuTypes[gpuType] = anyVal
	}

	return &GetAvailableGPUTypesResponse{
		GpuTypes: gpuTypes,
	}, nil
}

// Refresh triggers a refresh of the cached cloud provider state.
func (s *BareMetalProviderServer) Refresh(ctx context.Context, req *RefreshRequest) (*RefreshResponse, error) {
	// For bare metal, we don't maintain a cache - we always query the
	// Kubernetes API directly. This is a no-op but returns success.
	return &RefreshResponse{}, nil
}

// Cleanup performs any necessary cleanup when the autoscaler is shutting down.
func (s *BareMetalProviderServer) Cleanup(ctx context.Context, req *CleanupRequest) (*CleanupResponse, error) {
	// For bare metal, there's no external state to clean up.
	// The servers remain in their current state.
	return &CleanupResponse{}, nil
}

// Helper methods

// getMaxSize returns the maximum size of the node group (total number of servers).
func (s *BareMetalProviderServer) getMaxSize(ctx context.Context) int32 {
	var servers baremetalcontrollerv1.ServerList
	if err := s.Client.List(ctx, &servers); err != nil {
		return 0
	}
	return int32(len(servers.Items))
}

// mapPowerStateToInstanceState converts a server power state to an instance state.
func (s *BareMetalProviderServer) mapPowerStateToInstanceState(powerState baremetalcontrollerv1.PowerState) InstanceState {
	switch powerState {
	case baremetalcontrollerv1.PowerStateOn:
		return InstanceStatus_instanceRunning
	case baremetalcontrollerv1.PowerStateOff:
		return InstanceStatus_instanceDeleting
	default:
		return InstanceStatus_unspecified
	}
}
