package grpc

import (
	"context"
	"fmt"
	"net"
	"runtime/debug"

	"google.golang.org/grpc"
)

type Server interface {
	Run() error
	Stop() error
}

type GRPCHandlerRegister func(server *grpc.Server)

type grpcServer struct {
	network  string
	address  string
	handlers GRPCHandlerRegister
	server   *grpc.Server
}

func NewGRPCServer(network, address string, handlers GRPCHandlerRegister) Server {
	grpcser := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
				defer func() {
					r := recover()
					if r != nil {
						logger.Errorf("stacktrace from(%s) panic: \n%s", info.FullMethod, string(debug.Stack()))
						err = fmt.Errorf("%v", r)
						resp = nil
					}
				}()
				if req, ok := req.(interface {
					ValidateAll() error
				}); ok {
					err := req.ValidateAll()
					if err != nil {
						logger.Debugf("validate request %s for %+v error: %v", info.FullMethod, req, err)
						return nil, err
					}
				}
				return handler(ctx, req)
			},
		),
	)

	return &grpcServer{
		network:  network,
		address:  address,
		server:   grpcser,
		handlers: handlers,
	}
}

func (m *grpcServer) Run() error {
	m.handlers(m.server)
	l, err := net.Listen(m.network, m.address)
	if err != nil {
		return err
	}
	return m.server.Serve(l)
}

func (m *grpcServer) Stop() error {
	m.server.GracefulStop()
	return nil
}
