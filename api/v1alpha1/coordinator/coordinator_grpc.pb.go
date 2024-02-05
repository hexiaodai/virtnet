// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.19.4
// source: v1alpha1/coordinator/coordinator.proto

package coordinator

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// CoordinatorClient is the client API for Coordinator service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type CoordinatorClient interface {
	GetCoordinatorConfig(ctx context.Context, in *GetCoordinatorConfigRequest, opts ...grpc.CallOption) (*GetCoordinatorConfigReply, error)
}

type coordinatorClient struct {
	cc grpc.ClientConnInterface
}

func NewCoordinatorClient(cc grpc.ClientConnInterface) CoordinatorClient {
	return &coordinatorClient{cc}
}

func (c *coordinatorClient) GetCoordinatorConfig(ctx context.Context, in *GetCoordinatorConfigRequest, opts ...grpc.CallOption) (*GetCoordinatorConfigReply, error) {
	out := new(GetCoordinatorConfigReply)
	err := c.cc.Invoke(ctx, "/v1alpha1.coordinator.Coordinator/GetCoordinatorConfig", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CoordinatorServer is the server API for Coordinator service.
// All implementations should embed UnimplementedCoordinatorServer
// for forward compatibility
type CoordinatorServer interface {
	GetCoordinatorConfig(context.Context, *GetCoordinatorConfigRequest) (*GetCoordinatorConfigReply, error)
}

// UnimplementedCoordinatorServer should be embedded to have forward compatible implementations.
type UnimplementedCoordinatorServer struct {
}

func (UnimplementedCoordinatorServer) GetCoordinatorConfig(context.Context, *GetCoordinatorConfigRequest) (*GetCoordinatorConfigReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetCoordinatorConfig not implemented")
}

// UnsafeCoordinatorServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to CoordinatorServer will
// result in compilation errors.
type UnsafeCoordinatorServer interface {
	mustEmbedUnimplementedCoordinatorServer()
}

func RegisterCoordinatorServer(s grpc.ServiceRegistrar, srv CoordinatorServer) {
	s.RegisterService(&Coordinator_ServiceDesc, srv)
}

func _Coordinator_GetCoordinatorConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetCoordinatorConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CoordinatorServer).GetCoordinatorConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1alpha1.coordinator.Coordinator/GetCoordinatorConfig",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CoordinatorServer).GetCoordinatorConfig(ctx, req.(*GetCoordinatorConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Coordinator_ServiceDesc is the grpc.ServiceDesc for Coordinator service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Coordinator_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "v1alpha1.coordinator.Coordinator",
	HandlerType: (*CoordinatorServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetCoordinatorConfig",
			Handler:    _Coordinator_GetCoordinatorConfig_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "v1alpha1/coordinator/coordinator.proto",
}