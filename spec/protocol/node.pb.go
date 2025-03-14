// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.4
// 	protoc        v5.29.3
// source: spec/proto/node.proto

package protocol

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Node struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            uint64                 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	Address       string                 `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"`
	Unknown       bool                   `protobuf:"varint,3,opt,name=unknown,proto3" json:"unknown,omitempty"`       // indicate negotiation for ID is needed
	Rendezvous    bool                   `protobuf:"varint,4,opt,name=rendezvous,proto3" json:"rendezvous,omitempty"` // indicate a client connection
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Node) Reset() {
	*x = Node{}
	mi := &file_spec_proto_node_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Node) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Node) ProtoMessage() {}

func (x *Node) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_node_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Node.ProtoReflect.Descriptor instead.
func (*Node) Descriptor() ([]byte, []int) {
	return file_spec_proto_node_proto_rawDescGZIP(), []int{0}
}

func (x *Node) GetId() uint64 {
	if x != nil {
		return x.Id
	}
	return 0
}

func (x *Node) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

func (x *Node) GetUnknown() bool {
	if x != nil {
		return x.Unknown
	}
	return false
}

func (x *Node) GetRendezvous() bool {
	if x != nil {
		return x.Rendezvous
	}
	return false
}

var File_spec_proto_node_proto protoreflect.FileDescriptor

var file_spec_proto_node_proto_rawDesc = string([]byte{
	0x0a, 0x15, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x6e, 0x6f, 0x64,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f,
	0x6c, 0x22, 0x6a, 0x0a, 0x04, 0x4e, 0x6f, 0x64, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x02, 0x69, 0x64, 0x12, 0x18, 0x0a, 0x07, 0x61, 0x64, 0x64,
	0x72, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x61, 0x64, 0x64, 0x72,
	0x65, 0x73, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x75, 0x6e, 0x6b, 0x6e, 0x6f, 0x77, 0x6e, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x75, 0x6e, 0x6b, 0x6e, 0x6f, 0x77, 0x6e, 0x12, 0x1e, 0x0a,
	0x0a, 0x72, 0x65, 0x6e, 0x64, 0x65, 0x7a, 0x76, 0x6f, 0x75, 0x73, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x0a, 0x72, 0x65, 0x6e, 0x64, 0x65, 0x7a, 0x76, 0x6f, 0x75, 0x73, 0x42, 0x2b, 0x48,
	0x01, 0x5a, 0x27, 0x67, 0x6f, 0x2e, 0x6d, 0x69, 0x72, 0x61, 0x67, 0x65, 0x73, 0x70, 0x61, 0x63,
	0x65, 0x2e, 0x63, 0x6f, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x74, 0x65, 0x72, 0x2f, 0x73, 0x70, 0x65,
	0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
})

var (
	file_spec_proto_node_proto_rawDescOnce sync.Once
	file_spec_proto_node_proto_rawDescData []byte
)

func file_spec_proto_node_proto_rawDescGZIP() []byte {
	file_spec_proto_node_proto_rawDescOnce.Do(func() {
		file_spec_proto_node_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_spec_proto_node_proto_rawDesc), len(file_spec_proto_node_proto_rawDesc)))
	})
	return file_spec_proto_node_proto_rawDescData
}

var file_spec_proto_node_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_spec_proto_node_proto_goTypes = []any{
	(*Node)(nil), // 0: protocol.Node
}
var file_spec_proto_node_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_spec_proto_node_proto_init() }
func file_spec_proto_node_proto_init() {
	if File_spec_proto_node_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_spec_proto_node_proto_rawDesc), len(file_spec_proto_node_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_spec_proto_node_proto_goTypes,
		DependencyIndexes: file_spec_proto_node_proto_depIdxs,
		MessageInfos:      file_spec_proto_node_proto_msgTypes,
	}.Build()
	File_spec_proto_node_proto = out.File
	file_spec_proto_node_proto_goTypes = nil
	file_spec_proto_node_proto_depIdxs = nil
}
