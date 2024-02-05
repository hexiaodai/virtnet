package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime/debug"

	pbcoor "github.com/hexiaodai/virtnet/api/v1alpha1/coordinator"
	pbipam "github.com/hexiaodai/virtnet/api/v1alpha1/ipam"
	"github.com/hexiaodai/virtnet/pkg/barrel"
	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/coordinator"
	"github.com/hexiaodai/virtnet/pkg/endpoint"
	"github.com/hexiaodai/virtnet/pkg/ipam"
	"github.com/hexiaodai/virtnet/pkg/ippool"
	"github.com/hexiaodai/virtnet/pkg/pod"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	ctrl "sigs.k8s.io/controller-runtime"
)

var serverLogger *zap.SugaredLogger

type Server struct {
	mgr            ctrl.Manager
	ippoolClient   *ippool.IPPoolClient
	barrelClient   *barrel.BarrelClient
	podClient      *pod.PodClient
	endpointClient *endpoint.EndpointClient
	server         *grpc.Server
}

func NewServer(mgr ctrl.Manager, ippoolClient *ippool.IPPoolClient, barrelClient *barrel.BarrelClient, podClient *pod.PodClient, endpointClient *endpoint.EndpointClient) *Server {
	if serverLogger == nil {
		serverLogger = logger.Named("Server")
	}
	serverLogger.Debug("New Server")
	return &Server{
		server:         newGRPCServer(),
		mgr:            mgr,
		ippoolClient:   ippoolClient,
		barrelClient:   barrelClient,
		podClient:      podClient,
		endpointClient: endpointClient,
	}
}

func (s *Server) Run() error {
	serverLogger.Debug("Runing server")

	os.Remove(constant.DefaultUnixSocketPath)
	listen, err := net.Listen("unix", constant.DefaultUnixSocketPath)
	if err != nil {
		return errors.New("failed to listen: " + err.Error())
	}

	serverLogger.Debug("Register handler to server")
	s.registerToServer()

	serverLogger.Debugf("Server listening at %v", listen.Addr())
	if err := s.server.Serve(listen); err != nil {
		return fmt.Errorf("failed to serve '%v': %w", listen.Addr(), err)
	}

	return nil
}

func (s *Server) Stop() error {
	s.server.GracefulStop()
	return nil
}

func (s *Server) registerToServer() {
	pbipam.RegisterIpamServer(s.server, ipam.NewIPAM(s.ippoolClient, s.barrelClient, s.podClient, s.endpointClient))
	pbcoor.RegisterCoordinatorServer(s.server, coordinator.NewCoordinator(s.podClient))
}

func newGRPCServer() *grpc.Server {
	return grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
				defer func() {
					r := recover()
					if r != nil {
						serverLogger.Errorf("stacktrace from(%s) panic: \n%s", info.FullMethod, string(debug.Stack()))
						err = errors.New(fmt.Sprintf("%v", r))
						resp = nil
					}
				}()
				if req, ok := req.(interface {
					ValidateAll() error
				}); ok {
					err := req.ValidateAll()
					if err != nil {
						serverLogger.Debugf("validate request %s for %+v error: %v", info.FullMethod, req, err)
						return nil, err
					}
				}
				return handler(ctx, req)
			},
		),
	)
}
