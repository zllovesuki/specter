// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        v6.30.2
// source: spec/proto/client_query.proto

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

type ClientTunnel struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Hostname      string                 `protobuf:"bytes,1,opt,name=hostname,proto3" json:"hostname,omitempty"`
	Target        string                 `protobuf:"bytes,2,opt,name=target,proto3" json:"target,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClientTunnel) Reset() {
	*x = ClientTunnel{}
	mi := &file_spec_proto_client_query_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClientTunnel) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClientTunnel) ProtoMessage() {}

func (x *ClientTunnel) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_client_query_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClientTunnel.ProtoReflect.Descriptor instead.
func (*ClientTunnel) Descriptor() ([]byte, []int) {
	return file_spec_proto_client_query_proto_rawDescGZIP(), []int{0}
}

func (x *ClientTunnel) GetHostname() string {
	if x != nil {
		return x.Hostname
	}
	return ""
}

func (x *ClientTunnel) GetTarget() string {
	if x != nil {
		return x.Target
	}
	return ""
}

type ListTunnelsRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ListTunnelsRequest) Reset() {
	*x = ListTunnelsRequest{}
	mi := &file_spec_proto_client_query_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListTunnelsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListTunnelsRequest) ProtoMessage() {}

func (x *ListTunnelsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_client_query_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListTunnelsRequest.ProtoReflect.Descriptor instead.
func (*ListTunnelsRequest) Descriptor() ([]byte, []int) {
	return file_spec_proto_client_query_proto_rawDescGZIP(), []int{1}
}

type ListTunnelsResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Tunnels       []*ClientTunnel        `protobuf:"bytes,1,rep,name=tunnels,proto3" json:"tunnels,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ListTunnelsResponse) Reset() {
	*x = ListTunnelsResponse{}
	mi := &file_spec_proto_client_query_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListTunnelsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListTunnelsResponse) ProtoMessage() {}

func (x *ListTunnelsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_client_query_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListTunnelsResponse.ProtoReflect.Descriptor instead.
func (*ListTunnelsResponse) Descriptor() ([]byte, []int) {
	return file_spec_proto_client_query_proto_rawDescGZIP(), []int{2}
}

func (x *ListTunnelsResponse) GetTunnels() []*ClientTunnel {
	if x != nil {
		return x.Tunnels
	}
	return nil
}

var File_spec_proto_client_query_proto protoreflect.FileDescriptor

const file_spec_proto_client_query_proto_rawDesc = "" +
	"\n" +
	"\x1dspec/proto/client_query.proto\x12\bprotocol\"B\n" +
	"\fClientTunnel\x12\x1a\n" +
	"\bhostname\x18\x01 \x01(\tR\bhostname\x12\x16\n" +
	"\x06target\x18\x02 \x01(\tR\x06target\"\x14\n" +
	"\x12ListTunnelsRequest\"G\n" +
	"\x13ListTunnelsResponse\x120\n" +
	"\atunnels\x18\x01 \x03(\v2\x16.protocol.ClientTunnelR\atunnels2`\n" +
	"\x12ClientQueryService\x12J\n" +
	"\vListTunnels\x12\x1c.protocol.ListTunnelsRequest\x1a\x1d.protocol.ListTunnelsResponseB+H\x01Z'go.miragespace.co/specter/spec/protocolb\x06proto3"

var (
	file_spec_proto_client_query_proto_rawDescOnce sync.Once
	file_spec_proto_client_query_proto_rawDescData []byte
)

func file_spec_proto_client_query_proto_rawDescGZIP() []byte {
	file_spec_proto_client_query_proto_rawDescOnce.Do(func() {
		file_spec_proto_client_query_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_spec_proto_client_query_proto_rawDesc), len(file_spec_proto_client_query_proto_rawDesc)))
	})
	return file_spec_proto_client_query_proto_rawDescData
}

var file_spec_proto_client_query_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_spec_proto_client_query_proto_goTypes = []any{
	(*ClientTunnel)(nil),        // 0: protocol.ClientTunnel
	(*ListTunnelsRequest)(nil),  // 1: protocol.ListTunnelsRequest
	(*ListTunnelsResponse)(nil), // 2: protocol.ListTunnelsResponse
}
var file_spec_proto_client_query_proto_depIdxs = []int32{
	0, // 0: protocol.ListTunnelsResponse.tunnels:type_name -> protocol.ClientTunnel
	1, // 1: protocol.ClientQueryService.ListTunnels:input_type -> protocol.ListTunnelsRequest
	2, // 2: protocol.ClientQueryService.ListTunnels:output_type -> protocol.ListTunnelsResponse
	2, // [2:3] is the sub-list for method output_type
	1, // [1:2] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_spec_proto_client_query_proto_init() }
func file_spec_proto_client_query_proto_init() {
	if File_spec_proto_client_query_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_spec_proto_client_query_proto_rawDesc), len(file_spec_proto_client_query_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_spec_proto_client_query_proto_goTypes,
		DependencyIndexes: file_spec_proto_client_query_proto_depIdxs,
		MessageInfos:      file_spec_proto_client_query_proto_msgTypes,
	}.Build()
	File_spec_proto_client_query_proto = out.File
	file_spec_proto_client_query_proto_goTypes = nil
	file_spec_proto_client_query_proto_depIdxs = nil
}
