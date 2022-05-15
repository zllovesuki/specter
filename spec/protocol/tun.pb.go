// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.0
// 	protoc        v3.6.1
// source: spec/protocol/tun.proto

package protocol

import (
	descriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
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

type Link_ALPN int32

const (
	Link_UNKNOWN Link_ALPN = 0
	Link_SPECTER Link_ALPN = 1
	Link_HTTP    Link_ALPN = 10
	Link_TCP     Link_ALPN = 11
)

// Enum value maps for Link_ALPN.
var (
	Link_ALPN_name = map[int32]string{
		0:  "UNKNOWN",
		1:  "SPECTER",
		10: "HTTP",
		11: "TCP",
	}
	Link_ALPN_value = map[string]int32{
		"UNKNOWN": 0,
		"SPECTER": 1,
		"HTTP":    10,
		"TCP":     11,
	}
)

func (x Link_ALPN) Enum() *Link_ALPN {
	p := new(Link_ALPN)
	*p = x
	return p
}

func (x Link_ALPN) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Link_ALPN) Descriptor() protoreflect.EnumDescriptor {
	return file_spec_protocol_tun_proto_enumTypes[0].Descriptor()
}

func (Link_ALPN) Type() protoreflect.EnumType {
	return &file_spec_protocol_tun_proto_enumTypes[0]
}

func (x Link_ALPN) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Link_ALPN.Descriptor instead.
func (Link_ALPN) EnumDescriptor() ([]byte, []int) {
	return file_spec_protocol_tun_proto_rawDescGZIP(), []int{5, 0}
}

type Tunnel struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Client   *Node  `protobuf:"bytes,1,opt,name=client,proto3" json:"client,omitempty"`
	Server   *Node  `protobuf:"bytes,2,opt,name=server,proto3" json:"server,omitempty"`
	Hostname string `protobuf:"bytes,10,opt,name=hostname,proto3" json:"hostname,omitempty"`
}

func (x *Tunnel) Reset() {
	*x = Tunnel{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_protocol_tun_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Tunnel) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Tunnel) ProtoMessage() {}

func (x *Tunnel) ProtoReflect() protoreflect.Message {
	mi := &file_spec_protocol_tun_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Tunnel.ProtoReflect.Descriptor instead.
func (*Tunnel) Descriptor() ([]byte, []int) {
	return file_spec_protocol_tun_proto_rawDescGZIP(), []int{0}
}

func (x *Tunnel) GetClient() *Node {
	if x != nil {
		return x.Client
	}
	return nil
}

func (x *Tunnel) GetServer() *Node {
	if x != nil {
		return x.Server
	}
	return nil
}

func (x *Tunnel) GetHostname() string {
	if x != nil {
		return x.Hostname
	}
	return ""
}

type GetNodesRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *GetNodesRequest) Reset() {
	*x = GetNodesRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_protocol_tun_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetNodesRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetNodesRequest) ProtoMessage() {}

func (x *GetNodesRequest) ProtoReflect() protoreflect.Message {
	mi := &file_spec_protocol_tun_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetNodesRequest.ProtoReflect.Descriptor instead.
func (*GetNodesRequest) Descriptor() ([]byte, []int) {
	return file_spec_protocol_tun_proto_rawDescGZIP(), []int{1}
}

type GetNodesResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Nodes []*Node `protobuf:"bytes,1,rep,name=nodes,proto3" json:"nodes,omitempty"`
}

func (x *GetNodesResponse) Reset() {
	*x = GetNodesResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_protocol_tun_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetNodesResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetNodesResponse) ProtoMessage() {}

func (x *GetNodesResponse) ProtoReflect() protoreflect.Message {
	mi := &file_spec_protocol_tun_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetNodesResponse.ProtoReflect.Descriptor instead.
func (*GetNodesResponse) Descriptor() ([]byte, []int) {
	return file_spec_protocol_tun_proto_rawDescGZIP(), []int{2}
}

func (x *GetNodesResponse) GetNodes() []*Node {
	if x != nil {
		return x.Nodes
	}
	return nil
}

type PublishTunnelRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Client  *Node   `protobuf:"bytes,1,opt,name=client,proto3" json:"client,omitempty"`
	Servers []*Node `protobuf:"bytes,2,rep,name=servers,proto3" json:"servers,omitempty"`
}

func (x *PublishTunnelRequest) Reset() {
	*x = PublishTunnelRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_protocol_tun_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PublishTunnelRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PublishTunnelRequest) ProtoMessage() {}

func (x *PublishTunnelRequest) ProtoReflect() protoreflect.Message {
	mi := &file_spec_protocol_tun_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PublishTunnelRequest.ProtoReflect.Descriptor instead.
func (*PublishTunnelRequest) Descriptor() ([]byte, []int) {
	return file_spec_protocol_tun_proto_rawDescGZIP(), []int{3}
}

func (x *PublishTunnelRequest) GetClient() *Node {
	if x != nil {
		return x.Client
	}
	return nil
}

func (x *PublishTunnelRequest) GetServers() []*Node {
	if x != nil {
		return x.Servers
	}
	return nil
}

type PublishTunnelResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Hostname string `protobuf:"bytes,1,opt,name=hostname,proto3" json:"hostname,omitempty"`
}

func (x *PublishTunnelResponse) Reset() {
	*x = PublishTunnelResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_protocol_tun_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PublishTunnelResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PublishTunnelResponse) ProtoMessage() {}

func (x *PublishTunnelResponse) ProtoReflect() protoreflect.Message {
	mi := &file_spec_protocol_tun_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PublishTunnelResponse.ProtoReflect.Descriptor instead.
func (*PublishTunnelResponse) Descriptor() ([]byte, []int) {
	return file_spec_protocol_tun_proto_rawDescGZIP(), []int{4}
}

func (x *PublishTunnelResponse) GetHostname() string {
	if x != nil {
		return x.Hostname
	}
	return ""
}

type Link struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Alpn Link_ALPN `protobuf:"varint,1,opt,name=alpn,proto3,enum=protocol.Link_ALPN" json:"alpn,omitempty"`
}

func (x *Link) Reset() {
	*x = Link{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_protocol_tun_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Link) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Link) ProtoMessage() {}

func (x *Link) ProtoReflect() protoreflect.Message {
	mi := &file_spec_protocol_tun_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Link.ProtoReflect.Descriptor instead.
func (*Link) Descriptor() ([]byte, []int) {
	return file_spec_protocol_tun_proto_rawDescGZIP(), []int{5}
}

func (x *Link) GetAlpn() Link_ALPN {
	if x != nil {
		return x.Alpn
	}
	return Link_UNKNOWN
}

var file_spec_protocol_tun_proto_extTypes = []protoimpl.ExtensionInfo{
	{
		ExtendedType:  (*descriptor.EnumValueOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         54242,
		Name:          "protocol.alpn_name",
		Tag:           "bytes,54242,opt,name=alpn_name",
		Filename:      "spec/protocol/tun.proto",
	},
}

// Extension fields to descriptor.EnumValueOptions.
var (
	// optional string alpn_name = 54242;
	E_AlpnName = &file_spec_protocol_tun_proto_extTypes[0]
)

var File_spec_protocol_tun_proto protoreflect.FileDescriptor

var file_spec_protocol_tun_proto_rawDesc = []byte{
	0x0a, 0x17, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2f,
	0x74, 0x75, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x63, 0x6f, 0x6c, 0x1a, 0x18, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63,
	0x6f, 0x6c, 0x2f, 0x6e, 0x6f, 0x64, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x20, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64,
	0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22,
	0x74, 0x0a, 0x06, 0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x12, 0x26, 0x0a, 0x06, 0x63, 0x6c, 0x69,
	0x65, 0x6e, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4e, 0x6f, 0x64, 0x65, 0x52, 0x06, 0x63, 0x6c, 0x69, 0x65, 0x6e,
	0x74, 0x12, 0x26, 0x0a, 0x06, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x0e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4e, 0x6f, 0x64,
	0x65, 0x52, 0x06, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x12, 0x1a, 0x0a, 0x08, 0x68, 0x6f, 0x73,
	0x74, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x68, 0x6f, 0x73,
	0x74, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0x11, 0x0a, 0x0f, 0x47, 0x65, 0x74, 0x4e, 0x6f, 0x64, 0x65,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x38, 0x0a, 0x10, 0x47, 0x65, 0x74, 0x4e,
	0x6f, 0x64, 0x65, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x24, 0x0a, 0x05,
	0x6e, 0x6f, 0x64, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4e, 0x6f, 0x64, 0x65, 0x52, 0x05, 0x6e, 0x6f, 0x64,
	0x65, 0x73, 0x22, 0x68, 0x0a, 0x14, 0x50, 0x75, 0x62, 0x6c, 0x69, 0x73, 0x68, 0x54, 0x75, 0x6e,
	0x6e, 0x65, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x26, 0x0a, 0x06, 0x63, 0x6c,
	0x69, 0x65, 0x6e, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4e, 0x6f, 0x64, 0x65, 0x52, 0x06, 0x63, 0x6c, 0x69, 0x65,
	0x6e, 0x74, 0x12, 0x28, 0x0a, 0x07, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x18, 0x02, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4e,
	0x6f, 0x64, 0x65, 0x52, 0x07, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x22, 0x33, 0x0a, 0x15,
	0x50, 0x75, 0x62, 0x6c, 0x69, 0x73, 0x68, 0x54, 0x75, 0x6e, 0x6e, 0x65, 0x6c, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x68, 0x6f, 0x73, 0x74, 0x6e, 0x61, 0x6d,
	0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x68, 0x6f, 0x73, 0x74, 0x6e, 0x61, 0x6d,
	0x65, 0x22, 0x8a, 0x01, 0x0a, 0x04, 0x4c, 0x69, 0x6e, 0x6b, 0x12, 0x27, 0x0a, 0x04, 0x61, 0x6c,
	0x70, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x13, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x63, 0x6f, 0x6c, 0x2e, 0x4c, 0x69, 0x6e, 0x6b, 0x2e, 0x41, 0x4c, 0x50, 0x4e, 0x52, 0x04, 0x61,
	0x6c, 0x70, 0x6e, 0x22, 0x59, 0x0a, 0x04, 0x41, 0x4c, 0x50, 0x4e, 0x12, 0x0b, 0x0a, 0x07, 0x55,
	0x4e, 0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x10, 0x00, 0x12, 0x1a, 0x0a, 0x07, 0x53, 0x50, 0x45, 0x43,
	0x54, 0x45, 0x52, 0x10, 0x01, 0x1a, 0x0d, 0x92, 0xbe, 0x1a, 0x09, 0x73, 0x70, 0x65, 0x63, 0x74,
	0x65, 0x72, 0x2f, 0x31, 0x12, 0x16, 0x0a, 0x04, 0x48, 0x54, 0x54, 0x50, 0x10, 0x0a, 0x1a, 0x0c,
	0x92, 0xbe, 0x1a, 0x08, 0x68, 0x74, 0x74, 0x70, 0x2f, 0x31, 0x2e, 0x31, 0x12, 0x10, 0x0a, 0x03,
	0x54, 0x43, 0x50, 0x10, 0x0b, 0x1a, 0x07, 0x92, 0xbe, 0x1a, 0x03, 0x74, 0x63, 0x70, 0x3a, 0x40,
	0x0a, 0x09, 0x61, 0x6c, 0x70, 0x6e, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x21, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6e,
	0x75, 0x6d, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xe2,
	0xa7, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x61, 0x6c, 0x70, 0x6e, 0x4e, 0x61, 0x6d, 0x65,
	0x42, 0x19, 0x48, 0x01, 0x5a, 0x15, 0x73, 0x70, 0x65, 0x63, 0x74, 0x65, 0x72, 0x2f, 0x73, 0x70,
	0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x33,
}

var (
	file_spec_protocol_tun_proto_rawDescOnce sync.Once
	file_spec_protocol_tun_proto_rawDescData = file_spec_protocol_tun_proto_rawDesc
)

func file_spec_protocol_tun_proto_rawDescGZIP() []byte {
	file_spec_protocol_tun_proto_rawDescOnce.Do(func() {
		file_spec_protocol_tun_proto_rawDescData = protoimpl.X.CompressGZIP(file_spec_protocol_tun_proto_rawDescData)
	})
	return file_spec_protocol_tun_proto_rawDescData
}

var file_spec_protocol_tun_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_spec_protocol_tun_proto_msgTypes = make([]protoimpl.MessageInfo, 6)
var file_spec_protocol_tun_proto_goTypes = []interface{}{
	(Link_ALPN)(0),                      // 0: protocol.Link.ALPN
	(*Tunnel)(nil),                      // 1: protocol.Tunnel
	(*GetNodesRequest)(nil),             // 2: protocol.GetNodesRequest
	(*GetNodesResponse)(nil),            // 3: protocol.GetNodesResponse
	(*PublishTunnelRequest)(nil),        // 4: protocol.PublishTunnelRequest
	(*PublishTunnelResponse)(nil),       // 5: protocol.PublishTunnelResponse
	(*Link)(nil),                        // 6: protocol.Link
	(*Node)(nil),                        // 7: protocol.Node
	(*descriptor.EnumValueOptions)(nil), // 8: google.protobuf.EnumValueOptions
}
var file_spec_protocol_tun_proto_depIdxs = []int32{
	7, // 0: protocol.Tunnel.client:type_name -> protocol.Node
	7, // 1: protocol.Tunnel.server:type_name -> protocol.Node
	7, // 2: protocol.GetNodesResponse.nodes:type_name -> protocol.Node
	7, // 3: protocol.PublishTunnelRequest.client:type_name -> protocol.Node
	7, // 4: protocol.PublishTunnelRequest.servers:type_name -> protocol.Node
	0, // 5: protocol.Link.alpn:type_name -> protocol.Link.ALPN
	8, // 6: protocol.alpn_name:extendee -> google.protobuf.EnumValueOptions
	7, // [7:7] is the sub-list for method output_type
	7, // [7:7] is the sub-list for method input_type
	7, // [7:7] is the sub-list for extension type_name
	6, // [6:7] is the sub-list for extension extendee
	0, // [0:6] is the sub-list for field type_name
}

func init() { file_spec_protocol_tun_proto_init() }
func file_spec_protocol_tun_proto_init() {
	if File_spec_protocol_tun_proto != nil {
		return
	}
	file_spec_protocol_node_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_spec_protocol_tun_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Tunnel); i {
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
		file_spec_protocol_tun_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetNodesRequest); i {
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
		file_spec_protocol_tun_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetNodesResponse); i {
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
		file_spec_protocol_tun_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PublishTunnelRequest); i {
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
		file_spec_protocol_tun_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PublishTunnelResponse); i {
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
		file_spec_protocol_tun_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Link); i {
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
			RawDescriptor: file_spec_protocol_tun_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   6,
			NumExtensions: 1,
			NumServices:   0,
		},
		GoTypes:           file_spec_protocol_tun_proto_goTypes,
		DependencyIndexes: file_spec_protocol_tun_proto_depIdxs,
		EnumInfos:         file_spec_protocol_tun_proto_enumTypes,
		MessageInfos:      file_spec_protocol_tun_proto_msgTypes,
		ExtensionInfos:    file_spec_protocol_tun_proto_extTypes,
	}.Build()
	File_spec_protocol_tun_proto = out.File
	file_spec_protocol_tun_proto_rawDesc = nil
	file_spec_protocol_tun_proto_goTypes = nil
	file_spec_protocol_tun_proto_depIdxs = nil
}
