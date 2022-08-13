// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.6.1
// source: spec/proto/kv.proto

package protocol

import (
	duration "github.com/golang/protobuf/ptypes/duration"
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

type KVOperation int32

const (
	KVOperation_UNKNOWN_OP      KVOperation = 0
	KVOperation_SIMPLE_PUT      KVOperation = 1
	KVOperation_SIMPLE_GET      KVOperation = 2
	KVOperation_SIMPLE_DELETE   KVOperation = 3
	KVOperation_PREFIX_APPEND   KVOperation = 5
	KVOperation_PREFIX_LIST     KVOperation = 6
	KVOperation_PREFIX_REMOVE   KVOperation = 7
	KVOperation_PREFIX_CONTAINS KVOperation = 8
	KVOperation_LEASE_ACQUIRE   KVOperation = 10
	KVOperation_LEASE_RENEWAL   KVOperation = 11
	KVOperation_LEASE_RELEASE   KVOperation = 12
	KVOperation_IMPORT          KVOperation = 20
)

// Enum value maps for KVOperation.
var (
	KVOperation_name = map[int32]string{
		0:  "UNKNOWN_OP",
		1:  "SIMPLE_PUT",
		2:  "SIMPLE_GET",
		3:  "SIMPLE_DELETE",
		5:  "PREFIX_APPEND",
		6:  "PREFIX_LIST",
		7:  "PREFIX_REMOVE",
		8:  "PREFIX_CONTAINS",
		10: "LEASE_ACQUIRE",
		11: "LEASE_RENEWAL",
		12: "LEASE_RELEASE",
		20: "IMPORT",
	}
	KVOperation_value = map[string]int32{
		"UNKNOWN_OP":      0,
		"SIMPLE_PUT":      1,
		"SIMPLE_GET":      2,
		"SIMPLE_DELETE":   3,
		"PREFIX_APPEND":   5,
		"PREFIX_LIST":     6,
		"PREFIX_REMOVE":   7,
		"PREFIX_CONTAINS": 8,
		"LEASE_ACQUIRE":   10,
		"LEASE_RENEWAL":   11,
		"LEASE_RELEASE":   12,
		"IMPORT":          20,
	}
)

func (x KVOperation) Enum() *KVOperation {
	p := new(KVOperation)
	*p = x
	return p
}

func (x KVOperation) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (KVOperation) Descriptor() protoreflect.EnumDescriptor {
	return file_spec_proto_kv_proto_enumTypes[0].Descriptor()
}

func (KVOperation) Type() protoreflect.EnumType {
	return &file_spec_proto_kv_proto_enumTypes[0]
}

func (x KVOperation) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use KVOperation.Descriptor instead.
func (KVOperation) EnumDescriptor() ([]byte, []int) {
	return file_spec_proto_kv_proto_rawDescGZIP(), []int{0}
}

type KVTransfer struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	SimpleValue    []byte   `protobuf:"bytes,1,opt,name=simple_value,json=simpleValue,proto3" json:"simple_value,omitempty"`
	PrefixChildren [][]byte `protobuf:"bytes,2,rep,name=prefix_children,json=prefixChildren,proto3" json:"prefix_children,omitempty"`
	LeaseToken     uint64   `protobuf:"varint,3,opt,name=lease_token,json=leaseToken,proto3" json:"lease_token,omitempty"`
}

func (x *KVTransfer) Reset() {
	*x = KVTransfer{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_kv_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *KVTransfer) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*KVTransfer) ProtoMessage() {}

func (x *KVTransfer) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_kv_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use KVTransfer.ProtoReflect.Descriptor instead.
func (*KVTransfer) Descriptor() ([]byte, []int) {
	return file_spec_proto_kv_proto_rawDescGZIP(), []int{0}
}

func (x *KVTransfer) GetSimpleValue() []byte {
	if x != nil {
		return x.SimpleValue
	}
	return nil
}

func (x *KVTransfer) GetPrefixChildren() [][]byte {
	if x != nil {
		return x.PrefixChildren
	}
	return nil
}

func (x *KVTransfer) GetLeaseToken() uint64 {
	if x != nil {
		return x.LeaseToken
	}
	return 0
}

type KVLease struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ttl   *duration.Duration `protobuf:"bytes,1,opt,name=ttl,proto3" json:"ttl,omitempty"`
	Token uint64             `protobuf:"varint,2,opt,name=token,proto3" json:"token,omitempty"`
}

func (x *KVLease) Reset() {
	*x = KVLease{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_kv_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *KVLease) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*KVLease) ProtoMessage() {}

func (x *KVLease) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_kv_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use KVLease.ProtoReflect.Descriptor instead.
func (*KVLease) Descriptor() ([]byte, []int) {
	return file_spec_proto_kv_proto_rawDescGZIP(), []int{1}
}

func (x *KVLease) GetTtl() *duration.Duration {
	if x != nil {
		return x.Ttl
	}
	return nil
}

func (x *KVLease) GetToken() uint64 {
	if x != nil {
		return x.Token
	}
	return 0
}

type KVRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Op     KVOperation   `protobuf:"varint,1,opt,name=op,proto3,enum=protocol.KVOperation" json:"op,omitempty"`
	Key    []byte        `protobuf:"bytes,2,opt,name=key,proto3" json:"key,omitempty"`
	Value  []byte        `protobuf:"bytes,3,opt,name=value,proto3" json:"value,omitempty"`
	Lease  *KVLease      `protobuf:"bytes,5,opt,name=lease,proto3" json:"lease,omitempty"`
	Keys   [][]byte      `protobuf:"bytes,10,rep,name=keys,proto3" json:"keys,omitempty"`
	Values []*KVTransfer `protobuf:"bytes,20,rep,name=values,proto3" json:"values,omitempty"`
}

func (x *KVRequest) Reset() {
	*x = KVRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_kv_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *KVRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*KVRequest) ProtoMessage() {}

func (x *KVRequest) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_kv_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use KVRequest.ProtoReflect.Descriptor instead.
func (*KVRequest) Descriptor() ([]byte, []int) {
	return file_spec_proto_kv_proto_rawDescGZIP(), []int{2}
}

func (x *KVRequest) GetOp() KVOperation {
	if x != nil {
		return x.Op
	}
	return KVOperation_UNKNOWN_OP
}

func (x *KVRequest) GetKey() []byte {
	if x != nil {
		return x.Key
	}
	return nil
}

func (x *KVRequest) GetValue() []byte {
	if x != nil {
		return x.Value
	}
	return nil
}

func (x *KVRequest) GetLease() *KVLease {
	if x != nil {
		return x.Lease
	}
	return nil
}

func (x *KVRequest) GetKeys() [][]byte {
	if x != nil {
		return x.Keys
	}
	return nil
}

func (x *KVRequest) GetValues() []*KVTransfer {
	if x != nil {
		return x.Values
	}
	return nil
}

type KVResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Op     KVOperation   `protobuf:"varint,1,opt,name=op,proto3,enum=protocol.KVOperation" json:"op,omitempty"`
	Value  []byte        `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
	Lease  *KVLease      `protobuf:"bytes,5,opt,name=lease,proto3" json:"lease,omitempty"`
	Keys   [][]byte      `protobuf:"bytes,10,rep,name=keys,proto3" json:"keys,omitempty"`
	Values []*KVTransfer `protobuf:"bytes,20,rep,name=values,proto3" json:"values,omitempty"`
}

func (x *KVResponse) Reset() {
	*x = KVResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_kv_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *KVResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*KVResponse) ProtoMessage() {}

func (x *KVResponse) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_kv_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use KVResponse.ProtoReflect.Descriptor instead.
func (*KVResponse) Descriptor() ([]byte, []int) {
	return file_spec_proto_kv_proto_rawDescGZIP(), []int{3}
}

func (x *KVResponse) GetOp() KVOperation {
	if x != nil {
		return x.Op
	}
	return KVOperation_UNKNOWN_OP
}

func (x *KVResponse) GetValue() []byte {
	if x != nil {
		return x.Value
	}
	return nil
}

func (x *KVResponse) GetLease() *KVLease {
	if x != nil {
		return x.Lease
	}
	return nil
}

func (x *KVResponse) GetKeys() [][]byte {
	if x != nil {
		return x.Keys
	}
	return nil
}

func (x *KVResponse) GetValues() []*KVTransfer {
	if x != nil {
		return x.Values
	}
	return nil
}

var File_spec_proto_kv_proto protoreflect.FileDescriptor

var file_spec_proto_kv_proto_rawDesc = []byte{
	0x0a, 0x13, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x6b, 0x76, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x1a,
	0x1e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2f, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22,
	0x79, 0x0a, 0x0a, 0x4b, 0x56, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x66, 0x65, 0x72, 0x12, 0x21, 0x0a,
	0x0c, 0x73, 0x69, 0x6d, 0x70, 0x6c, 0x65, 0x5f, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0c, 0x52, 0x0b, 0x73, 0x69, 0x6d, 0x70, 0x6c, 0x65, 0x56, 0x61, 0x6c, 0x75, 0x65,
	0x12, 0x27, 0x0a, 0x0f, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x5f, 0x63, 0x68, 0x69, 0x6c, 0x64,
	0x72, 0x65, 0x6e, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x0e, 0x70, 0x72, 0x65, 0x66, 0x69,
	0x78, 0x43, 0x68, 0x69, 0x6c, 0x64, 0x72, 0x65, 0x6e, 0x12, 0x1f, 0x0a, 0x0b, 0x6c, 0x65, 0x61,
	0x73, 0x65, 0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x04, 0x52, 0x0a,
	0x6c, 0x65, 0x61, 0x73, 0x65, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x22, 0x4c, 0x0a, 0x07, 0x4b, 0x56,
	0x4c, 0x65, 0x61, 0x73, 0x65, 0x12, 0x2b, 0x0a, 0x03, 0x74, 0x74, 0x6c, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x03, 0x74,
	0x74, 0x6c, 0x12, 0x14, 0x0a, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x04, 0x52, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x22, 0xc5, 0x01, 0x0a, 0x09, 0x4b, 0x56, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x25, 0x0a, 0x02, 0x6f, 0x70, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0e, 0x32, 0x15, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4b, 0x56,
	0x4f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x02, 0x6f, 0x70, 0x12, 0x10, 0x0a,
	0x03, 0x6b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12,
	0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x05,
	0x76, 0x61, 0x6c, 0x75, 0x65, 0x12, 0x27, 0x0a, 0x05, 0x6c, 0x65, 0x61, 0x73, 0x65, 0x18, 0x05,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x11, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e,
	0x4b, 0x56, 0x4c, 0x65, 0x61, 0x73, 0x65, 0x52, 0x05, 0x6c, 0x65, 0x61, 0x73, 0x65, 0x12, 0x12,
	0x0a, 0x04, 0x6b, 0x65, 0x79, 0x73, 0x18, 0x0a, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x04, 0x6b, 0x65,
	0x79, 0x73, 0x12, 0x2c, 0x0a, 0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73, 0x18, 0x14, 0x20, 0x03,
	0x28, 0x0b, 0x32, 0x14, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4b, 0x56,
	0x54, 0x72, 0x61, 0x6e, 0x73, 0x66, 0x65, 0x72, 0x52, 0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73,
	0x22, 0xb4, 0x01, 0x0a, 0x0a, 0x4b, 0x56, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x25, 0x0a, 0x02, 0x6f, 0x70, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x15, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4b, 0x56, 0x4f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x52, 0x02, 0x6f, 0x70, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x12, 0x27, 0x0a, 0x05,
	0x6c, 0x65, 0x61, 0x73, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x11, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4b, 0x56, 0x4c, 0x65, 0x61, 0x73, 0x65, 0x52, 0x05,
	0x6c, 0x65, 0x61, 0x73, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x6b, 0x65, 0x79, 0x73, 0x18, 0x0a, 0x20,
	0x03, 0x28, 0x0c, 0x52, 0x04, 0x6b, 0x65, 0x79, 0x73, 0x12, 0x2c, 0x0a, 0x06, 0x76, 0x61, 0x6c,
	0x75, 0x65, 0x73, 0x18, 0x14, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4b, 0x56, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x66, 0x65, 0x72, 0x52,
	0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73, 0x2a, 0xe1, 0x01, 0x0a, 0x0b, 0x4b, 0x56, 0x4f, 0x70,
	0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x0e, 0x0a, 0x0a, 0x55, 0x4e, 0x4b, 0x4e, 0x4f,
	0x57, 0x4e, 0x5f, 0x4f, 0x50, 0x10, 0x00, 0x12, 0x0e, 0x0a, 0x0a, 0x53, 0x49, 0x4d, 0x50, 0x4c,
	0x45, 0x5f, 0x50, 0x55, 0x54, 0x10, 0x01, 0x12, 0x0e, 0x0a, 0x0a, 0x53, 0x49, 0x4d, 0x50, 0x4c,
	0x45, 0x5f, 0x47, 0x45, 0x54, 0x10, 0x02, 0x12, 0x11, 0x0a, 0x0d, 0x53, 0x49, 0x4d, 0x50, 0x4c,
	0x45, 0x5f, 0x44, 0x45, 0x4c, 0x45, 0x54, 0x45, 0x10, 0x03, 0x12, 0x11, 0x0a, 0x0d, 0x50, 0x52,
	0x45, 0x46, 0x49, 0x58, 0x5f, 0x41, 0x50, 0x50, 0x45, 0x4e, 0x44, 0x10, 0x05, 0x12, 0x0f, 0x0a,
	0x0b, 0x50, 0x52, 0x45, 0x46, 0x49, 0x58, 0x5f, 0x4c, 0x49, 0x53, 0x54, 0x10, 0x06, 0x12, 0x11,
	0x0a, 0x0d, 0x50, 0x52, 0x45, 0x46, 0x49, 0x58, 0x5f, 0x52, 0x45, 0x4d, 0x4f, 0x56, 0x45, 0x10,
	0x07, 0x12, 0x13, 0x0a, 0x0f, 0x50, 0x52, 0x45, 0x46, 0x49, 0x58, 0x5f, 0x43, 0x4f, 0x4e, 0x54,
	0x41, 0x49, 0x4e, 0x53, 0x10, 0x08, 0x12, 0x11, 0x0a, 0x0d, 0x4c, 0x45, 0x41, 0x53, 0x45, 0x5f,
	0x41, 0x43, 0x51, 0x55, 0x49, 0x52, 0x45, 0x10, 0x0a, 0x12, 0x11, 0x0a, 0x0d, 0x4c, 0x45, 0x41,
	0x53, 0x45, 0x5f, 0x52, 0x45, 0x4e, 0x45, 0x57, 0x41, 0x4c, 0x10, 0x0b, 0x12, 0x11, 0x0a, 0x0d,
	0x4c, 0x45, 0x41, 0x53, 0x45, 0x5f, 0x52, 0x45, 0x4c, 0x45, 0x41, 0x53, 0x45, 0x10, 0x0c, 0x12,
	0x0a, 0x0a, 0x06, 0x49, 0x4d, 0x50, 0x4f, 0x52, 0x54, 0x10, 0x14, 0x42, 0x25, 0x48, 0x01, 0x5a,
	0x21, 0x6b, 0x6f, 0x6e, 0x2e, 0x6e, 0x65, 0x63, 0x74, 0x2e, 0x73, 0x68, 0x2f, 0x73, 0x70, 0x65,
	0x63, 0x74, 0x65, 0x72, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63,
	0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_spec_proto_kv_proto_rawDescOnce sync.Once
	file_spec_proto_kv_proto_rawDescData = file_spec_proto_kv_proto_rawDesc
)

func file_spec_proto_kv_proto_rawDescGZIP() []byte {
	file_spec_proto_kv_proto_rawDescOnce.Do(func() {
		file_spec_proto_kv_proto_rawDescData = protoimpl.X.CompressGZIP(file_spec_proto_kv_proto_rawDescData)
	})
	return file_spec_proto_kv_proto_rawDescData
}

var file_spec_proto_kv_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_spec_proto_kv_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_spec_proto_kv_proto_goTypes = []interface{}{
	(KVOperation)(0),          // 0: protocol.KVOperation
	(*KVTransfer)(nil),        // 1: protocol.KVTransfer
	(*KVLease)(nil),           // 2: protocol.KVLease
	(*KVRequest)(nil),         // 3: protocol.KVRequest
	(*KVResponse)(nil),        // 4: protocol.KVResponse
	(*duration.Duration)(nil), // 5: google.protobuf.Duration
}
var file_spec_proto_kv_proto_depIdxs = []int32{
	5, // 0: protocol.KVLease.ttl:type_name -> google.protobuf.Duration
	0, // 1: protocol.KVRequest.op:type_name -> protocol.KVOperation
	2, // 2: protocol.KVRequest.lease:type_name -> protocol.KVLease
	1, // 3: protocol.KVRequest.values:type_name -> protocol.KVTransfer
	0, // 4: protocol.KVResponse.op:type_name -> protocol.KVOperation
	2, // 5: protocol.KVResponse.lease:type_name -> protocol.KVLease
	1, // 6: protocol.KVResponse.values:type_name -> protocol.KVTransfer
	7, // [7:7] is the sub-list for method output_type
	7, // [7:7] is the sub-list for method input_type
	7, // [7:7] is the sub-list for extension type_name
	7, // [7:7] is the sub-list for extension extendee
	0, // [0:7] is the sub-list for field type_name
}

func init() { file_spec_proto_kv_proto_init() }
func file_spec_proto_kv_proto_init() {
	if File_spec_proto_kv_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_spec_proto_kv_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*KVTransfer); i {
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
		file_spec_proto_kv_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*KVLease); i {
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
		file_spec_proto_kv_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*KVRequest); i {
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
		file_spec_proto_kv_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*KVResponse); i {
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
			RawDescriptor: file_spec_proto_kv_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_spec_proto_kv_proto_goTypes,
		DependencyIndexes: file_spec_proto_kv_proto_depIdxs,
		EnumInfos:         file_spec_proto_kv_proto_enumTypes,
		MessageInfos:      file_spec_proto_kv_proto_msgTypes,
	}.Build()
	File_spec_proto_kv_proto = out.File
	file_spec_proto_kv_proto_rawDesc = nil
	file_spec_proto_kv_proto_goTypes = nil
	file_spec_proto_kv_proto_depIdxs = nil
}
