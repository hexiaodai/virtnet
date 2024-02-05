package cmd

import (
	pbcoor "github.com/hexiaodai/virtnet/api/v1alpha1/coordinator"
	pbipam "github.com/hexiaodai/virtnet/api/v1alpha1/ipam"
	"github.com/hexiaodai/virtnet/pkg/coordinator"
	"github.com/hexiaodai/virtnet/pkg/ipam"
	"google.golang.org/grpc"
)

func RegisterGRPCHandler(crdmgr *CRDManager) func(s *grpc.Server) {
	return func(s *grpc.Server) {
		pbipam.RegisterIpamServer(s, ipam.NewIPAM(crdmgr.IPPoolClient, crdmgr.PodClient, crdmgr.EndpointClient))
		pbcoor.RegisterCoordinatorServer(s, coordinator.NewCoordinator(crdmgr.PodClient))
	}
}
