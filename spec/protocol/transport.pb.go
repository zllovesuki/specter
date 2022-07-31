// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.6.1
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
)

// Enum value maps for Stream_Type.
var (
	Stream_Type_name = map[int32]string{
		0: "UNKNOWN_TYPE",
		1: "RPC",
		2: "DIRECT",
	}
	Stream_Type_value = map[string]int32{
		"UNKNOWN_TYPE": 0,
		"RPC":          1,
		"DIRECT":       2,
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
)

// Enum value maps for Datagram_Type.
var (
	Datagram_Type_name = map[int32]string{
		0: "UNKNOWN",
		1: "ALIVE",
		2: "DATA",
	}
	Datagram_Type_value = map[string]int32{
		"UNKNOWN": 0,
		"ALIVE":   1,
		"DATA":    2,
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

type Stream struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type Stream_Type `protobuf:"varint,1,opt,name=type,proto3,enum=protocol.Stream_Type" json:"type,omitempty"`
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

	Identity *Node `protobuf:"bytes,1,opt,name=identity,proto3" json:"identity,omitempty"`
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

var File_spec_proto_transport_proto protoreflect.FileDescriptor

var file_spec_proto_transport_proto_rawDesc = []byte{
	0x0a, 0x1a, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x74, 0x72, 0x61,
	0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x1a, 0x15, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x2f, 0x6e, 0x6f, 0x64, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x62, 0x0a,
	0x06, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x12, 0x29, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x15, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c,
	0x2e, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79,
	0x70, 0x65, 0x22, 0x2d, 0x0a, 0x04, 0x54, 0x79, 0x70, 0x65, 0x12, 0x10, 0x0a, 0x0c, 0x55, 0x4e,
	0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x10, 0x00, 0x12, 0x07, 0x0a, 0x03,
	0x52, 0x50, 0x43, 0x10, 0x01, 0x12, 0x0a, 0x0a, 0x06, 0x44, 0x49, 0x52, 0x45, 0x43, 0x54, 0x10,
	0x02, 0x22, 0x75, 0x0a, 0x08, 0x44, 0x61, 0x74, 0x61, 0x67, 0x72, 0x61, 0x6d, 0x12, 0x2b, 0x0a,
	0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x17, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x44, 0x61, 0x74, 0x61, 0x67, 0x72, 0x61, 0x6d, 0x2e,
	0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x64, 0x61,
	0x74, 0x61, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x64, 0x61, 0x74, 0x61, 0x22, 0x28,
	0x0a, 0x04, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0b, 0x0a, 0x07, 0x55, 0x4e, 0x4b, 0x4e, 0x4f, 0x57,
	0x4e, 0x10, 0x00, 0x12, 0x09, 0x0a, 0x05, 0x41, 0x4c, 0x49, 0x56, 0x45, 0x10, 0x01, 0x12, 0x08,
	0x0a, 0x04, 0x44, 0x41, 0x54, 0x41, 0x10, 0x02, 0x22, 0x38, 0x0a, 0x0a, 0x43, 0x6f, 0x6e, 0x6e,
	0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x2a, 0x0a, 0x08, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69,
	0x74, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x63, 0x6f, 0x6c, 0x2e, 0x4e, 0x6f, 0x64, 0x65, 0x52, 0x08, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69,
	0x74, 0x79, 0x42, 0x25, 0x48, 0x01, 0x5a, 0x21, 0x6b, 0x6f, 0x6e, 0x2e, 0x6e, 0x65, 0x63, 0x74,
	0x2e, 0x73, 0x68, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x74, 0x65, 0x72, 0x2f, 0x73, 0x70, 0x65, 0x63,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
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

var file_spec_proto_transport_proto_enumTypes = make([]protoimpl.EnumInfo, 2)
var file_spec_proto_transport_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_spec_proto_transport_proto_goTypes = []interface{}{
	(Stream_Type)(0),   // 0: protocol.Stream.Type
	(Datagram_Type)(0), // 1: protocol.Datagram.Type
	(*Stream)(nil),     // 2: protocol.Stream
	(*Datagram)(nil),   // 3: protocol.Datagram
	(*Connection)(nil), // 4: protocol.Connection
	(*Node)(nil),       // 5: protocol.Node
}
var file_spec_proto_transport_proto_depIdxs = []int32{
	0, // 0: protocol.Stream.type:type_name -> protocol.Stream.Type
	1, // 1: protocol.Datagram.type:type_name -> protocol.Datagram.Type
	5, // 2: protocol.Connection.identity:type_name -> protocol.Node
	3, // [3:3] is the sub-list for method output_type
	3, // [3:3] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
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
			NumEnums:      2,
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
