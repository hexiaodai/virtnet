// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v3.19.4
// source: v1alpha1/coordinator/coordinator.proto

package coordinator

import (
	_ "github.com/envoyproxy/protoc-gen-validate/validate"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type GetCoordinatorConfigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// pod name
	PodName string `protobuf:"bytes,1,opt,name=PodName,proto3" json:"PodName,omitempty"`
	// pod namespace
	PodNamespace string `protobuf:"bytes,2,opt,name=PodNamespace,proto3" json:"PodNamespace,omitempty"`
}

func (x *GetCoordinatorConfigRequest) Reset() {
	*x = GetCoordinatorConfigRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1alpha1_coordinator_coordinator_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetCoordinatorConfigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetCoordinatorConfigRequest) ProtoMessage() {}

func (x *GetCoordinatorConfigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_v1alpha1_coordinator_coordinator_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetCoordinatorConfigRequest.ProtoReflect.Descriptor instead.
func (*GetCoordinatorConfigRequest) Descriptor() ([]byte, []int) {
	return file_v1alpha1_coordinator_coordinator_proto_rawDescGZIP(), []int{0}
}

func (x *GetCoordinatorConfigRequest) GetPodName() string {
	if x != nil {
		return x.PodName
	}
	return ""
}

func (x *GetCoordinatorConfigRequest) GetPodNamespace() string {
	if x != nil {
		return x.PodNamespace
	}
	return ""
}

type GetCoordinatorConfigReply struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// service CIDR
	ServiceCIDR []string `protobuf:"bytes,1,rep,name=ServiceCIDR,proto3" json:"ServiceCIDR,omitempty"`
}

func (x *GetCoordinatorConfigReply) Reset() {
	*x = GetCoordinatorConfigReply{}
	if protoimpl.UnsafeEnabled {
		mi := &file_v1alpha1_coordinator_coordinator_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetCoordinatorConfigReply) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetCoordinatorConfigReply) ProtoMessage() {}

func (x *GetCoordinatorConfigReply) ProtoReflect() protoreflect.Message {
	mi := &file_v1alpha1_coordinator_coordinator_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetCoordinatorConfigReply.ProtoReflect.Descriptor instead.
func (*GetCoordinatorConfigReply) Descriptor() ([]byte, []int) {
	return file_v1alpha1_coordinator_coordinator_proto_rawDescGZIP(), []int{1}
}

func (x *GetCoordinatorConfigReply) GetServiceCIDR() []string {
	if x != nil {
		return x.ServiceCIDR
	}
	return nil
}

var File_v1alpha1_coordinator_coordinator_proto protoreflect.FileDescriptor

var file_v1alpha1_coordinator_coordinator_proto_rawDesc = []byte{
	0x0a, 0x26, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2f, 0x63, 0x6f, 0x6f, 0x72, 0x64,
	0x69, 0x6e, 0x61, 0x74, 0x6f, 0x72, 0x2f, 0x63, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61, 0x74,
	0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x14, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68,
	0x61, 0x31, 0x2e, 0x63, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61, 0x74, 0x6f, 0x72, 0x1a, 0x23,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x76, 0x65, 0x6e, 0x64, 0x6f, 0x72, 0x2f, 0x76, 0x61, 0x6c, 0x69,
	0x64, 0x61, 0x74, 0x65, 0x2f, 0x76, 0x61, 0x6c, 0x69, 0x64, 0x61, 0x74, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x22, 0x6d, 0x0a, 0x1b, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6f, 0x72, 0x64, 0x69,
	0x6e, 0x61, 0x74, 0x6f, 0x72, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x12, 0x21, 0x0a, 0x07, 0x50, 0x6f, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x42, 0x07, 0xfa, 0x42, 0x04, 0x72, 0x02, 0x10, 0x01, 0x52, 0x07, 0x50, 0x6f,
	0x64, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x2b, 0x0a, 0x0c, 0x50, 0x6f, 0x64, 0x4e, 0x61, 0x6d, 0x65,
	0x73, 0x70, 0x61, 0x63, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x42, 0x07, 0xfa, 0x42, 0x04,
	0x72, 0x02, 0x10, 0x01, 0x52, 0x0c, 0x50, 0x6f, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61,
	0x63, 0x65, 0x22, 0x3d, 0x0a, 0x19, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e,
	0x61, 0x74, 0x6f, 0x72, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x12,
	0x20, 0x0a, 0x0b, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x43, 0x49, 0x44, 0x52, 0x18, 0x01,
	0x20, 0x03, 0x28, 0x09, 0x52, 0x0b, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x43, 0x49, 0x44,
	0x52, 0x32, 0x8b, 0x01, 0x0a, 0x0b, 0x43, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61, 0x74, 0x6f,
	0x72, 0x12, 0x7c, 0x0a, 0x14, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61,
	0x74, 0x6f, 0x72, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x31, 0x2e, 0x76, 0x31, 0x61, 0x6c,
	0x70, 0x68, 0x61, 0x31, 0x2e, 0x63, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61, 0x74, 0x6f, 0x72,
	0x2e, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61, 0x74, 0x6f, 0x72, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2f, 0x2e, 0x76,
	0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2e, 0x63, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61,
	0x74, 0x6f, 0x72, 0x2e, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61, 0x74,
	0x6f, 0x72, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x22, 0x00, 0x42,
	0x16, 0x5a, 0x14, 0x76, 0x31, 0x61, 0x6c, 0x70, 0x68, 0x61, 0x31, 0x2f, 0x63, 0x6f, 0x6f, 0x72,
	0x64, 0x69, 0x6e, 0x61, 0x74, 0x6f, 0x72, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_v1alpha1_coordinator_coordinator_proto_rawDescOnce sync.Once
	file_v1alpha1_coordinator_coordinator_proto_rawDescData = file_v1alpha1_coordinator_coordinator_proto_rawDesc
)

func file_v1alpha1_coordinator_coordinator_proto_rawDescGZIP() []byte {
	file_v1alpha1_coordinator_coordinator_proto_rawDescOnce.Do(func() {
		file_v1alpha1_coordinator_coordinator_proto_rawDescData = protoimpl.X.CompressGZIP(file_v1alpha1_coordinator_coordinator_proto_rawDescData)
	})
	return file_v1alpha1_coordinator_coordinator_proto_rawDescData
}

var file_v1alpha1_coordinator_coordinator_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_v1alpha1_coordinator_coordinator_proto_goTypes = []interface{}{
	(*GetCoordinatorConfigRequest)(nil), // 0: v1alpha1.coordinator.GetCoordinatorConfigRequest
	(*GetCoordinatorConfigReply)(nil),   // 1: v1alpha1.coordinator.GetCoordinatorConfigReply
}
var file_v1alpha1_coordinator_coordinator_proto_depIdxs = []int32{
	0, // 0: v1alpha1.coordinator.Coordinator.GetCoordinatorConfig:input_type -> v1alpha1.coordinator.GetCoordinatorConfigRequest
	1, // 1: v1alpha1.coordinator.Coordinator.GetCoordinatorConfig:output_type -> v1alpha1.coordinator.GetCoordinatorConfigReply
	1, // [1:2] is the sub-list for method output_type
	0, // [0:1] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_v1alpha1_coordinator_coordinator_proto_init() }
func file_v1alpha1_coordinator_coordinator_proto_init() {
	if File_v1alpha1_coordinator_coordinator_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_v1alpha1_coordinator_coordinator_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetCoordinatorConfigRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_v1alpha1_coordinator_coordinator_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetCoordinatorConfigReply); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_v1alpha1_coordinator_coordinator_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_v1alpha1_coordinator_coordinator_proto_goTypes,
		DependencyIndexes: file_v1alpha1_coordinator_coordinator_proto_depIdxs,
		MessageInfos:      file_v1alpha1_coordinator_coordinator_proto_msgTypes,
	}.Build()
	File_v1alpha1_coordinator_coordinator_proto = out.File
	file_v1alpha1_coordinator_coordinator_proto_rawDesc = nil
	file_v1alpha1_coordinator_coordinator_proto_goTypes = nil
	file_v1alpha1_coordinator_coordinator_proto_depIdxs = nil
}
