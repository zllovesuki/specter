// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.6.1
// source: kv/aof/proto/mutation.proto

package proto

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	protocol "kon.nect.sh/specter/spec/protocol"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type MutationType int32

const (
	MutationType_UNKNOWN_TYPE  MutationType = 0
	MutationType_SIMPLE_PUT    MutationType = 1
	MutationType_SIMPLE_DELETE MutationType = 3
	MutationType_PREFIX_APPEND MutationType = 5
	MutationType_PREFIX_REMOVE MutationType = 7
	MutationType_IMPORT        MutationType = 20
	MutationType_REMOVE_KEYS   MutationType = 21
)

// Enum value maps for MutationType.
var (
	MutationType_name = map[int32]string{
		0:  "UNKNOWN_TYPE",
		1:  "SIMPLE_PUT",
		3:  "SIMPLE_DELETE",
		5:  "PREFIX_APPEND",
		7:  "PREFIX_REMOVE",
		20: "IMPORT",
		21: "REMOVE_KEYS",
	}
	MutationType_value = map[string]int32{
		"UNKNOWN_TYPE":  0,
		"SIMPLE_PUT":    1,
		"SIMPLE_DELETE": 3,
		"PREFIX_APPEND": 5,
		"PREFIX_REMOVE": 7,
		"IMPORT":        20,
		"REMOVE_KEYS":   21,
	}
)

func (x MutationType) Enum() *MutationType {
	p := new(MutationType)
	*p = x
	return p
}

func (x MutationType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (MutationType) Descriptor() protoreflect.EnumDescriptor {
	return file_kv_aof_proto_mutation_proto_enumTypes[0].Descriptor()
}

func (MutationType) Type() protoreflect.EnumType {
	return &file_kv_aof_proto_mutation_proto_enumTypes[0]
}

func (x MutationType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use MutationType.Descriptor instead.
func (MutationType) EnumDescriptor() ([]byte, []int) {
	return file_kv_aof_proto_mutation_proto_rawDescGZIP(), []int{0}
}

type Mutation struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type   MutationType           `protobuf:"varint,1,opt,name=type,proto3,enum=proto.MutationType" json:"type,omitempty"`
	Key    []byte                 `protobuf:"bytes,2,opt,name=key,proto3" json:"key,omitempty"`
	Value  []byte                 `protobuf:"bytes,3,opt,name=value,proto3" json:"value,omitempty"`
	Keys   [][]byte               `protobuf:"bytes,10,rep,name=keys,proto3" json:"keys,omitempty"`
	Values []*protocol.KVTransfer `protobuf:"bytes,20,rep,name=values,proto3" json:"values,omitempty"`
}

func (x *Mutation) Reset() {
	*x = Mutation{}
	if protoimpl.UnsafeEnabled {
		mi := &file_kv_aof_proto_mutation_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Mutation) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Mutation) ProtoMessage() {}

func (x *Mutation) ProtoReflect() protoreflect.Message {
	mi := &file_kv_aof_proto_mutation_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Mutation.ProtoReflect.Descriptor instead.
func (*Mutation) Descriptor() ([]byte, []int) {
	return file_kv_aof_proto_mutation_proto_rawDescGZIP(), []int{0}
}

func (x *Mutation) GetType() MutationType {
	if x != nil {
		return x.Type
	}
	return MutationType_UNKNOWN_TYPE
}

func (x *Mutation) GetKey() []byte {
	if x != nil {
		return x.Key
	}
	return nil
}

func (x *Mutation) GetValue() []byte {
	if x != nil {
		return x.Value
	}
	return nil
}

func (x *Mutation) GetKeys() [][]byte {
	if x != nil {
		return x.Keys
	}
	return nil
}

func (x *Mutation) GetValues() []*protocol.KVTransfer {
	if x != nil {
		return x.Values
	}
	return nil
}

var File_kv_aof_proto_mutation_proto protoreflect.FileDescriptor

var file_kv_aof_proto_mutation_proto_rawDesc = []byte{
	0x0a, 0x1b, 0x6b, 0x76, 0x2f, 0x61, 0x6f, 0x66, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x6d,
	0x75, 0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x13, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x6b, 0x76, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x9d, 0x01, 0x0a, 0x08, 0x4d, 0x75,
	0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x27, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0e, 0x32, 0x13, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x4d, 0x75, 0x74,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12,
	0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x6b, 0x65,
	0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c,
	0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x6b, 0x65, 0x79, 0x73, 0x18,
	0x0a, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x04, 0x6b, 0x65, 0x79, 0x73, 0x12, 0x2c, 0x0a, 0x06, 0x76,
	0x61, 0x6c, 0x75, 0x65, 0x73, 0x18, 0x14, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4b, 0x56, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x66, 0x65,
	0x72, 0x52, 0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73, 0x2a, 0x86, 0x01, 0x0a, 0x0c, 0x4d, 0x75,
	0x74, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x54, 0x79, 0x70, 0x65, 0x12, 0x10, 0x0a, 0x0c, 0x55, 0x4e,
	0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x10, 0x00, 0x12, 0x0e, 0x0a, 0x0a,
	0x53, 0x49, 0x4d, 0x50, 0x4c, 0x45, 0x5f, 0x50, 0x55, 0x54, 0x10, 0x01, 0x12, 0x11, 0x0a, 0x0d,
	0x53, 0x49, 0x4d, 0x50, 0x4c, 0x45, 0x5f, 0x44, 0x45, 0x4c, 0x45, 0x54, 0x45, 0x10, 0x03, 0x12,
	0x11, 0x0a, 0x0d, 0x50, 0x52, 0x45, 0x46, 0x49, 0x58, 0x5f, 0x41, 0x50, 0x50, 0x45, 0x4e, 0x44,
	0x10, 0x05, 0x12, 0x11, 0x0a, 0x0d, 0x50, 0x52, 0x45, 0x46, 0x49, 0x58, 0x5f, 0x52, 0x45, 0x4d,
	0x4f, 0x56, 0x45, 0x10, 0x07, 0x12, 0x0a, 0x0a, 0x06, 0x49, 0x4d, 0x50, 0x4f, 0x52, 0x54, 0x10,
	0x14, 0x12, 0x0f, 0x0a, 0x0b, 0x52, 0x45, 0x4d, 0x4f, 0x56, 0x45, 0x5f, 0x4b, 0x45, 0x59, 0x53,
	0x10, 0x15, 0x42, 0x24, 0x48, 0x01, 0x5a, 0x20, 0x6b, 0x6f, 0x6e, 0x2e, 0x6e, 0x65, 0x63, 0x74,
	0x2e, 0x73, 0x68, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x74, 0x65, 0x72, 0x2f, 0x6b, 0x76, 0x2f, 0x61,
	0x6f, 0x66, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_kv_aof_proto_mutation_proto_rawDescOnce sync.Once
	file_kv_aof_proto_mutation_proto_rawDescData = file_kv_aof_proto_mutation_proto_rawDesc
)

func file_kv_aof_proto_mutation_proto_rawDescGZIP() []byte {
	file_kv_aof_proto_mutation_proto_rawDescOnce.Do(func() {
		file_kv_aof_proto_mutation_proto_rawDescData = protoimpl.X.CompressGZIP(file_kv_aof_proto_mutation_proto_rawDescData)
	})
	return file_kv_aof_proto_mutation_proto_rawDescData
}

var file_kv_aof_proto_mutation_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_kv_aof_proto_mutation_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_kv_aof_proto_mutation_proto_goTypes = []interface{}{
	(MutationType)(0),           // 0: proto.MutationType
	(*Mutation)(nil),            // 1: proto.Mutation
	(*protocol.KVTransfer)(nil), // 2: protocol.KVTransfer
}
var file_kv_aof_proto_mutation_proto_depIdxs = []int32{
	0, // 0: proto.Mutation.type:type_name -> proto.MutationType
	2, // 1: proto.Mutation.values:type_name -> protocol.KVTransfer
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_kv_aof_proto_mutation_proto_init() }
func file_kv_aof_proto_mutation_proto_init() {
	if File_kv_aof_proto_mutation_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_kv_aof_proto_mutation_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Mutation); i {
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
			RawDescriptor: file_kv_aof_proto_mutation_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_kv_aof_proto_mutation_proto_goTypes,
		DependencyIndexes: file_kv_aof_proto_mutation_proto_depIdxs,
		EnumInfos:         file_kv_aof_proto_mutation_proto_enumTypes,
		MessageInfos:      file_kv_aof_proto_mutation_proto_msgTypes,
	}.Build()
	File_kv_aof_proto_mutation_proto = out.File
	file_kv_aof_proto_mutation_proto_rawDesc = nil
	file_kv_aof_proto_mutation_proto_goTypes = nil
	file_kv_aof_proto_mutation_proto_depIdxs = nil
}