// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.0
// 	protoc        v3.6.1
// source: spec/protocol/transport.proto

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
	Stream_TUNNEL       Stream_Type = 2
)

// Enum value maps for Stream_Type.
var (
	Stream_Type_name = map[int32]string{
		0: "UNKNOWN_TYPE",
		1: "RPC",
		2: "TUNNEL",
	}
	Stream_Type_value = map[string]int32{
		"UNKNOWN_TYPE": 0,
		"RPC":          1,
		"TUNNEL":       2,
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
	return file_spec_protocol_transport_proto_enumTypes[0].Descriptor()
}

func (Stream_Type) Type() protoreflect.EnumType {
	return &file_spec_protocol_transport_proto_enumTypes[0]
}

func (x Stream_Type) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Stream_Type.Descriptor instead.
func (Stream_Type) EnumDescriptor() ([]byte, []int) {
	return file_spec_protocol_transport_proto_rawDescGZIP(), []int{0, 0}
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
		mi := &file_spec_protocol_transport_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Stream) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Stream) ProtoMessage() {}

func (x *Stream) ProtoReflect() protoreflect.Message {
	mi := &file_spec_protocol_transport_proto_msgTypes[0]
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
	return file_spec_protocol_transport_proto_rawDescGZIP(), []int{0}
}

func (x *Stream) GetType() Stream_Type {
	if x != nil {
		return x.Type
	}
	return Stream_UNKNOWN_TYPE
}

var File_spec_protocol_transport_proto protoreflect.FileDescriptor

var file_spec_protocol_transport_proto_rawDesc = []byte{
	0x0a, 0x1d, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2f,
	0x74, 0x72, 0x61, 0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12,
	0x08, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x22, 0x62, 0x0a, 0x06, 0x53, 0x74, 0x72,
	0x65, 0x61, 0x6d, 0x12, 0x29, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x0e, 0x32, 0x15, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x53, 0x74, 0x72,
	0x65, 0x61, 0x6d, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x22, 0x2d,
	0x0a, 0x04, 0x54, 0x79, 0x70, 0x65, 0x12, 0x10, 0x0a, 0x0c, 0x55, 0x4e, 0x4b, 0x4e, 0x4f, 0x57,
	0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x10, 0x00, 0x12, 0x07, 0x0a, 0x03, 0x52, 0x50, 0x43, 0x10,
	0x01, 0x12, 0x0a, 0x0a, 0x06, 0x54, 0x55, 0x4e, 0x4e, 0x45, 0x4c, 0x10, 0x02, 0x42, 0x19, 0x48,
	0x01, 0x5a, 0x15, 0x73, 0x70, 0x65, 0x63, 0x74, 0x65, 0x72, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_spec_protocol_transport_proto_rawDescOnce sync.Once
	file_spec_protocol_transport_proto_rawDescData = file_spec_protocol_transport_proto_rawDesc
)

func file_spec_protocol_transport_proto_rawDescGZIP() []byte {
	file_spec_protocol_transport_proto_rawDescOnce.Do(func() {
		file_spec_protocol_transport_proto_rawDescData = protoimpl.X.CompressGZIP(file_spec_protocol_transport_proto_rawDescData)
	})
	return file_spec_protocol_transport_proto_rawDescData
}

var file_spec_protocol_transport_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_spec_protocol_transport_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_spec_protocol_transport_proto_goTypes = []interface{}{
	(Stream_Type)(0), // 0: protocol.Stream.Type
	(*Stream)(nil),   // 1: protocol.Stream
}
var file_spec_protocol_transport_proto_depIdxs = []int32{
	0, // 0: protocol.Stream.type:type_name -> protocol.Stream.Type
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_spec_protocol_transport_proto_init() }
func file_spec_protocol_transport_proto_init() {
	if File_spec_protocol_transport_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_spec_protocol_transport_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
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
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_spec_protocol_transport_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_spec_protocol_transport_proto_goTypes,
		DependencyIndexes: file_spec_protocol_transport_proto_depIdxs,
		EnumInfos:         file_spec_protocol_transport_proto_enumTypes,
		MessageInfos:      file_spec_protocol_transport_proto_msgTypes,
	}.Build()
	File_spec_protocol_transport_proto = out.File
	file_spec_protocol_transport_proto_rawDesc = nil
	file_spec_protocol_transport_proto_goTypes = nil
	file_spec_protocol_transport_proto_depIdxs = nil
}
