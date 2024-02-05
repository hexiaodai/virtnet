package coordinator

import (
	"context"

	pb "github.com/hexiaodai/virtnet/api/v1alpha1/coordinator"
	"github.com/hexiaodai/virtnet/pkg/pod"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ pb.CoordinatorServer = (*coordinator)(nil)

type coordinator struct {
	podClient *pod.PodClient
}

func NewCoordinator(podClient *pod.PodClient) pb.CoordinatorServer {
	return &coordinator{
		podClient: podClient,
	}
}

func (coor *coordinator) GetCoordinatorConfig(ctx context.Context, request *pb.GetCoordinatorConfigRequest) (*pb.GetCoordinatorConfigReply, error) {
	if err := request.ValidateAll(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	logger := logger.Named("GetCoordinatorConfig").With(
		"CNICommand", "ADD",
		"PodNamespace", request.PodNamespace,
		"PodName", request.PodName,
	)

	logger.Debug("Start GetCoordinatorConfig")

	ipnet, err := coor.podClient.GetServiceClusterIPRange(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	logger.Debugw("'podClient.GetServiceClusterIPRange()'", "ServiceCIDR", ipnet.String())
	reply := &pb.GetCoordinatorConfigReply{
		ServiceCIDR: []string{ipnet.String()},
	}

	return reply, nil
}
