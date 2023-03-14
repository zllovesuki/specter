// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.29.0
// 	protoc        v4.22.2
// source: spec/proto/transport.proto

package protocol

import (
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

type Stream_Type int32

const (
	Stream_UNKNOWN_TYPE Stream_Type = 0
	Stream_RPC          Stream_Type = 1
	Stream_DIRECT       Stream_Type = 2
	Stream_PROXY        Stream_Type = 3
	Stream_INTERNAL     Stream_Type = 4
)

// Enum value maps for Stream_Type.
var (
	Stream_Type_name = map[int32]string{
		0: "UNKNOWN_TYPE",
		1: "RPC",
		2: "DIRECT",
		3: "PROXY",
		4: "INTERNAL",
	}
	Stream_Type_value = map[string]int32{
		"UNKNOWN_TYPE": 0,
		"RPC":          1,
		"DIRECT":       2,
		"PROXY":        3,
		"INTERNAL":     4,
	}
)

func (x Stream_Type) Enum() *Stream_Type {
	p := new(Stream_Type)
	*p = x
	return p
}

func (x Stream_Type) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Stream_Type) Descriptor() protoreflect.EnumDescriptor {
	return file_spec_proto_transport_proto_enumTypes[0].Descriptor()
}

func (Stream_Type) Type() protoreflect.EnumType {
	return &file_spec_proto_transport_proto_enumTypes[0]
}

func (x Stream_Type) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Stream_Type.Descriptor instead.
func (Stream_Type) EnumDescriptor() ([]byte, []int) {
	return file_spec_proto_transport_proto_rawDescGZIP(), []int{0, 0}
}

type Datagram_Type int32

const (
	Datagram_UNKNOWN Datagram_Type = 0
	Datagram_ALIVE   Datagram_Type = 1
	Datagram_DATA    Datagram_Type = 2
	Datagram_RTT_SYN Datagram_Type = 3
	Datagram_RTT_ACK Datagram_Type = 4
)

// Enum value maps for Datagram_Type.
var (
	Datagram_Type_name = map[int32]string{
		0: "UNKNOWN",
		1: "ALIVE",
		2: "DATA",
		3: "RTT_SYN",
		4: "RTT_ACK",
	}
	Datagram_Type_value = map[string]int32{
		"UNKNOWN": 0,
		"ALIVE":   1,
		"DATA":    2,
		"RTT_SYN": 3,
		"RTT_ACK": 4,
	}
)

func (x Datagram_Type) Enum() *Datagram_Type {
	p := new(Datagram_Type)
	*p = x
	return p
}

func (x Datagram_Type) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Datagram_Type) Descriptor() protoreflect.EnumDescriptor {
	return file_spec_proto_transport_proto_enumTypes[1].Descriptor()
}

func (Datagram_Type) Type() protoreflect.EnumType {
	return &file_spec_proto_transport_proto_enumTypes[1]
}

func (x Datagram_Type) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Datagram_Type.Descriptor instead.
func (Datagram_Type) EnumDescriptor() ([]byte, []int) {
	return file_spec_proto_transport_proto_rawDescGZIP(), []int{1, 0}
}

type Connection_State int32

const (
	Connection_UNKNOWN_STATE Connection_State = 0
	Connection_CACHED        Connection_State = 1
	Connection_FRESH         Connection_State = 2
)

// Enum value maps for Connection_State.
var (
	Connection_State_name = map[int32]string{
		0: "UNKNOWN_STATE",
		1: "CACHED",
		2: "FRESH",
	}
	Connection_State_value = map[string]int32{
		"UNKNOWN_STATE": 0,
		"CACHED":        1,
		"FRESH":         2,
	}
)

func (x Connection_State) Enum() *Connection_State {
	p := new(Connection_State)
	*p = x
	return p
}

func (x Connection_State) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Connection_State) Descriptor() protoreflect.EnumDescriptor {
	return file_spec_proto_transport_proto_enumTypes[2].Descriptor()
}

func (Connection_State) Type() protoreflect.EnumType {
	return &file_spec_proto_transport_proto_enumTypes[2]
}

func (x Connection_State) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Connection_State.Descriptor instead.
func (Connection_State) EnumDescriptor() ([]byte, []int) {
	return file_spec_proto_transport_proto_rawDescGZIP(), []int{2, 0}
}

type Connection_Direction int32

const (
	Connection_UNKNOWN_DIRECTION Connection_Direction = 0
	Connection_INCOMING          Connection_Direction = 1
	Connection_OUTGOING          Connection_Direction = 2
)

// Enum value maps for Connection_Direction.
var (
	Connection_Direction_name = map[int32]string{
		0: "UNKNOWN_DIRECTION",
		1: "INCOMING",
		2: "OUTGOING",
	}
	Connection_Direction_value = map[string]int32{
		"UNKNOWN_DIRECTION": 0,
		"INCOMING":          1,
		"OUTGOING":          2,
	}
)

func (x Connection_Direction) Enum() *Connection_Direction {
	p := new(Connection_Direction)
	*p = x
	return p
}

func (x Connection_Direction) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Connection_Direction) Descriptor() protoreflect.EnumDescriptor {
	return file_spec_proto_transport_proto_enumTypes[3].Descriptor()
}

func (Connection_Direction) Type() protoreflect.EnumType {
	return &file_spec_proto_transport_proto_enumTypes[3]
}

func (x Connection_Direction) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Connection_Direction.Descriptor instead.
func (Connection_Direction) EnumDescriptor() ([]byte, []int) {
	return file_spec_proto_transport_proto_rawDescGZIP(), []int{2, 1}
}

type Stream struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type   Stream_Type `protobuf:"varint,1,opt,name=type,proto3,enum=protocol.Stream_Type" json:"type,omitempty"`
	Target *Node       `protobuf:"bytes,2,opt,name=target,proto3" json:"target,omitempty"`
}

func (x *Stream) Reset() {
	*x = Stream{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_transport_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Stream) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Stream) ProtoMessage() {}

func (x *Stream) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_transport_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Stream.ProtoReflect.Descriptor instead.
func (*Stream) Descriptor() ([]byte, []int) {
	return file_spec_proto_transport_proto_rawDescGZIP(), []int{0}
}

func (x *Stream) GetType() Stream_Type {
	if x != nil {
		return x.Type
	}
	return Stream_UNKNOWN_TYPE
}

func (x *Stream) GetTarget() *Node {
	if x != nil {
		return x.Target
	}
	return nil
}

type Datagram struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type Datagram_Type `protobuf:"varint,1,opt,name=type,proto3,enum=protocol.Datagram_Type" json:"type,omitempty"`
	Data []byte        `protobuf:"bytes,2,opt,name=data,proto3" json:"data,omitempty"`
}

func (x *Datagram) Reset() {
	*x = Datagram{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_transport_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Datagram) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Datagram) ProtoMessage() {}

func (x *Datagram) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_transport_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Datagram.ProtoReflect.Descriptor instead.
func (*Datagram) Descriptor() ([]byte, []int) {
	return file_spec_proto_transport_proto_rawDescGZIP(), []int{1}
}

func (x *Datagram) GetType() Datagram_Type {
	if x != nil {
		return x.Type
	}
	return Datagram_UNKNOWN
}

func (x *Datagram) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

type Connection struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Identity       *Node                `protobuf:"bytes,1,opt,name=identity,proto3" json:"identity,omitempty"`
	CacheState     Connection_State     `protobuf:"varint,2,opt,name=cache_state,json=cacheState,proto3,enum=protocol.Connection_State" json:"cache_state,omitempty"`
	CacheDirection Connection_Direction `protobuf:"varint,3,opt,name=cache_direction,json=cacheDirection,proto3,enum=protocol.Connection_Direction" json:"cache_direction,omitempty"`
}

func (x *Connection) Reset() {
	*x = Connection{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_transport_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Connection) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Connection) ProtoMessage() {}

func (x *Connection) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_transport_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Connection.ProtoReflect.Descriptor instead.
func (*Connection) Descriptor() ([]byte, []int) {
	return file_spec_proto_transport_proto_rawDescGZIP(), []int{2}
}

func (x *Connection) GetIdentity() *Node {
	if x != nil {
		return x.Identity
	}
	return nil
}

func (x *Connection) GetCacheState() Connection_State {
	if x != nil {
		return x.CacheState
	}
	return Connection_UNKNOWN_STATE
}

func (x *Connection) GetCacheDirection() Connection_Direction {
	if x != nil {
		return x.CacheDirection
	}
	return Connection_UNKNOWN_DIRECTION
}

var File_spec_proto_transport_proto protoreflect.FileDescriptor

var file_spec_proto_transport_proto_rawDesc = []byte{
	0x0a, 0x1a, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x74, 0x72, 0x61,
	0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x1a, 0x15, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x2f, 0x6e, 0x6f, 0x64, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xa3, 0x01,
	0x0a, 0x06, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x12, 0x29, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x15, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f,
	0x6c, 0x2e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74,
	0x79, 0x70, 0x65, 0x12, 0x26, 0x0a, 0x06, 0x74, 0x61, 0x72, 0x67, 0x65, 0x74, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4e,
	0x6f, 0x64, 0x65, 0x52, 0x06, 0x74, 0x61, 0x72, 0x67, 0x65, 0x74, 0x22, 0x46, 0x0a, 0x04, 0x54,
	0x79, 0x70, 0x65, 0x12, 0x10, 0x0a, 0x0c, 0x55, 0x4e, 0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x5f, 0x54,
	0x59, 0x50, 0x45, 0x10, 0x00, 0x12, 0x07, 0x0a, 0x03, 0x52, 0x50, 0x43, 0x10, 0x01, 0x12, 0x0a,
	0x0a, 0x06, 0x44, 0x49, 0x52, 0x45, 0x43, 0x54, 0x10, 0x02, 0x12, 0x09, 0x0a, 0x05, 0x50, 0x52,
	0x4f, 0x58, 0x59, 0x10, 0x03, 0x12, 0x0c, 0x0a, 0x08, 0x49, 0x4e, 0x54, 0x45, 0x52, 0x4e, 0x41,
	0x4c, 0x10, 0x04, 0x22, 0x8f, 0x01, 0x0a, 0x08, 0x44, 0x61, 0x74, 0x61, 0x67, 0x72, 0x61, 0x6d,
	0x12, 0x2b, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x17,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x44, 0x61, 0x74, 0x61, 0x67, 0x72,
	0x61, 0x6d, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x12, 0x0a,
	0x04, 0x64, 0x61, 0x74, 0x61, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x64, 0x61, 0x74,
	0x61, 0x22, 0x42, 0x0a, 0x04, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0b, 0x0a, 0x07, 0x55, 0x4e, 0x4b,
	0x4e, 0x4f, 0x57, 0x4e, 0x10, 0x00, 0x12, 0x09, 0x0a, 0x05, 0x41, 0x4c, 0x49, 0x56, 0x45, 0x10,
	0x01, 0x12, 0x08, 0x0a, 0x04, 0x44, 0x41, 0x54, 0x41, 0x10, 0x02, 0x12, 0x0b, 0x0a, 0x07, 0x52,
	0x54, 0x54, 0x5f, 0x53, 0x59, 0x4e, 0x10, 0x03, 0x12, 0x0b, 0x0a, 0x07, 0x52, 0x54, 0x54, 0x5f,
	0x41, 0x43, 0x4b, 0x10, 0x04, 0x22, 0xb1, 0x02, 0x0a, 0x0a, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63,
	0x74, 0x69, 0x6f, 0x6e, 0x12, 0x2a, 0x0a, 0x08, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f,
	0x6c, 0x2e, 0x4e, 0x6f, 0x64, 0x65, 0x52, 0x08, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79,
	0x12, 0x3b, 0x0a, 0x0b, 0x63, 0x61, 0x63, 0x68, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x1a, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c,
	0x2e, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x53, 0x74, 0x61, 0x74,
	0x65, 0x52, 0x0a, 0x63, 0x61, 0x63, 0x68, 0x65, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x47, 0x0a,
	0x0f, 0x63, 0x61, 0x63, 0x68, 0x65, 0x5f, 0x64, 0x69, 0x72, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x1e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f,
	0x6c, 0x2e, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x44, 0x69, 0x72,
	0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x0e, 0x63, 0x61, 0x63, 0x68, 0x65, 0x44, 0x69, 0x72,
	0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x31, 0x0a, 0x05, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12,
	0x11, 0x0a, 0x0d, 0x55, 0x4e, 0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x5f, 0x53, 0x54, 0x41, 0x54, 0x45,
	0x10, 0x00, 0x12, 0x0a, 0x0a, 0x06, 0x43, 0x41, 0x43, 0x48, 0x45, 0x44, 0x10, 0x01, 0x12, 0x09,
	0x0a, 0x05, 0x46, 0x52, 0x45, 0x53, 0x48, 0x10, 0x02, 0x22, 0x3e, 0x0a, 0x09, 0x44, 0x69, 0x72,
	0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x15, 0x0a, 0x11, 0x55, 0x4e, 0x4b, 0x4e, 0x4f, 0x57,
	0x4e, 0x5f, 0x44, 0x49, 0x52, 0x45, 0x43, 0x54, 0x49, 0x4f, 0x4e, 0x10, 0x00, 0x12, 0x0c, 0x0a,
	0x08, 0x49, 0x4e, 0x43, 0x4f, 0x4d, 0x49, 0x4e, 0x47, 0x10, 0x01, 0x12, 0x0c, 0x0a, 0x08, 0x4f,
	0x55, 0x54, 0x47, 0x4f, 0x49, 0x4e, 0x47, 0x10, 0x02, 0x42, 0x25, 0x48, 0x01, 0x5a, 0x21, 0x6b,
	0x6f, 0x6e, 0x2e, 0x6e, 0x65, 0x63, 0x74, 0x2e, 0x73, 0x68, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x74,
	0x65, 0x72, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_spec_proto_transport_proto_rawDescOnce sync.Once
	file_spec_proto_transport_proto_rawDescData = file_spec_proto_transport_proto_rawDesc
)

func file_spec_proto_transport_proto_rawDescGZIP() []byte {
	file_spec_proto_transport_proto_rawDescOnce.Do(func() {
		file_spec_proto_transport_proto_rawDescData = protoimpl.X.CompressGZIP(file_spec_proto_transport_proto_rawDescData)
	})
	return file_spec_proto_transport_proto_rawDescData
}

var file_spec_proto_transport_proto_enumTypes = make([]protoimpl.EnumInfo, 4)
var file_spec_proto_transport_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_spec_proto_transport_proto_goTypes = []interface{}{
	(Stream_Type)(0),          // 0: protocol.Stream.Type
	(Datagram_Type)(0),        // 1: protocol.Datagram.Type
	(Connection_State)(0),     // 2: protocol.Connection.State
	(Connection_Direction)(0), // 3: protocol.Connection.Direction
	(*Stream)(nil),            // 4: protocol.Stream
	(*Datagram)(nil),          // 5: protocol.Datagram
	(*Connection)(nil),        // 6: protocol.Connection
	(*Node)(nil),              // 7: protocol.Node
}
var file_spec_proto_transport_proto_depIdxs = []int32{
	0, // 0: protocol.Stream.type:type_name -> protocol.Stream.Type
	7, // 1: protocol.Stream.target:type_name -> protocol.Node
	1, // 2: protocol.Datagram.type:type_name -> protocol.Datagram.Type
	7, // 3: protocol.Connection.identity:type_name -> protocol.Node
	2, // 4: protocol.Connection.cache_state:type_name -> protocol.Connection.State
	3, // 5: protocol.Connection.cache_direction:type_name -> protocol.Connection.Direction
	6, // [6:6] is the sub-list for method output_type
	6, // [6:6] is the sub-list for method input_type
	6, // [6:6] is the sub-list for extension type_name
	6, // [6:6] is the sub-list for extension extendee
	0, // [0:6] is the sub-list for field type_name
}

func init() { file_spec_proto_transport_proto_init() }
func file_spec_proto_transport_proto_init() {
	if File_spec_proto_transport_proto != nil {
		return
	}
	file_spec_proto_node_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_spec_proto_transport_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Stream); i {
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
		file_spec_proto_transport_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Datagram); i {
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
		file_spec_proto_transport_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Connection); i {
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
			RawDescriptor: file_spec_proto_transport_proto_rawDesc,
			NumEnums:      4,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_spec_proto_transport_proto_goTypes,
		DependencyIndexes: file_spec_proto_transport_proto_depIdxs,
		EnumInfos:         file_spec_proto_transport_proto_enumTypes,
		MessageInfos:      file_spec_proto_transport_proto_msgTypes,
	}.Build()
	File_spec_proto_transport_proto = out.File
	file_spec_proto_transport_proto_rawDesc = nil
	file_spec_proto_transport_proto_goTypes = nil
	file_spec_proto_transport_proto_depIdxs = nil
}
