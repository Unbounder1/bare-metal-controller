package protos

import (
	"context"

	baremetalcontrollerv1 "github.com/Unbounder1/bare-metal-controller/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BareMetalProviderServer struct {
	UnimplementedCloudProviderServer
	Client client.Client
}

const defaultNodeGroupID = "bare-metal-pool"

func (s *BareMetalProviderServer) NodeGroups(ctx context.Context, req *NodeGroupsRequest) (*NodeGroupsResponse, error) {

	var servers baremetalcontrollerv1.ServerList

	if err := s.Client.List(ctx, &servers); err != nil {
		return nil, err
	}

	// Current functionality: only support a single node group
	nodeGroups := make([]*NodeGroup, 1)

	nodeGroups[0] = &NodeGroup{
		Id:      defaultNodeGroupID,
		MinSize: 0,
		MaxSize: int32(len(servers.Items)),
	}

	return &NodeGroupsResponse{
		NodeGroups: nodeGroups,
	}, nil
}

func (s *BareMetalProviderServer) NodeGroupIncreaseSize(ctx context.Context, req *NodeGroupIncreaseSizeRequest) (*NodeGroupIncreaseSizeResponse, error) {
	nodeGroupID := req.GetId()

	// Currently only a single node group is supported
	if nodeGroupID != defaultNodeGroupID {
		return nil, nil
	}

	delta := req.GetDelta()

	// Provision any node in nodeGroup that is offline
	var servers baremetalcontrollerv1.ServerList
	if err := s.Client.List(ctx, &servers); err != nil {
		return nil, err
	}

	for i := 0; i < int(delta); i++ {
		for _, server := range servers.Items {
			if server.Spec.PowerState == baremetalcontrollerv1.PowerStateOff {
				server.Spec.PowerState = baremetalcontrollerv1.PowerStateOn
				if err := s.Client.Update(ctx, &server); err != nil {
					return nil, err
				}
				break
			}
		}
	}
	return &NodeGroupIncreaseSizeResponse{}, nil
}

func (s *BareMetalProviderServer) NodeGroupDeleteNodes(ctx context.Context, req *NodeGroupDeleteNodesRequest) (*NodeGroupDeleteNodesResponse, error) {
	// TODO: deprovision bare metal nodes
	return &NodeGroupDeleteNodesResponse{}, nil
}

func (s *BareMetalProviderServer) NodeGroupForNode(ctx context.Context, req *NodeGroupForNodeRequest) (*NodeGroupForNodeResponse, error) {
	// TODO: match node to node group
	return &NodeGroupForNodeResponse{}, nil
}

func (s *BareMetalProviderServer) NodeGroupTargetSize(ctx context.Context, req *NodeGroupTargetSizeRequest) (*NodeGroupTargetSizeResponse, error) {
	// TODO: return current target size
	return &NodeGroupTargetSizeResponse{}, nil
}

func (s *BareMetalProviderServer) NodeGroupDecreaseTargetSize(ctx context.Context, req *NodeGroupDecreaseTargetSizeRequest) (*NodeGroupDecreaseTargetSizeResponse, error) {
	// TODO: decrease target size
	return &NodeGroupDecreaseTargetSizeResponse{}, nil
}

func (s *BareMetalProviderServer) NodeGroupNodes(ctx context.Context, req *NodeGroupNodesRequest) (*NodeGroupNodesResponse, error) {
	// TODO: list nodes in group
	return &NodeGroupNodesResponse{}, nil
}

func (s *BareMetalProviderServer) GPULabel(ctx context.Context, req *GPULabelRequest) (*GPULabelResponse, error) {
	// TODO: return GPU label
	return &GPULabelResponse{}, nil
}

func (s *BareMetalProviderServer) GetAvailableGPUTypes(ctx context.Context, req *GetAvailableGPUTypesRequest) (*GetAvailableGPUTypesResponse, error) {
	// TODO: return available GPU types
	return &GetAvailableGPUTypesResponse{}, nil
}

func (s *BareMetalProviderServer) Refresh(ctx context.Context, req *RefreshRequest) (*RefreshResponse, error) {
	// TODO: refresh cached state
	return &RefreshResponse{}, nil
}

func (s *BareMetalProviderServer) Cleanup(ctx context.Context, req *CleanupRequest) (*CleanupResponse, error) {
	// TODO: cleanup resources
	return &CleanupResponse{}, nil
}
