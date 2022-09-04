// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.6.1
// source: spec/proto/rpc.proto

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

type Context_TargetType int32

const (
	Context_UNKNOWN        Context_TargetType = 0
	Context_KV_REPLICATION Context_TargetType = 1
)

// Enum value maps for Context_TargetType.
var (
	Context_TargetType_name = map[int32]string{
		0: "UNKNOWN",
		1: "KV_REPLICATION",
	}
	Context_TargetType_value = map[string]int32{
		"UNKNOWN":        0,
		"KV_REPLICATION": 1,
	}
)

func (x Context_TargetType) Enum() *Context_TargetType {
	p := new(Context_TargetType)
	*p = x
	return p
}

func (x Context_TargetType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Context_TargetType) Descriptor() protoreflect.EnumDescriptor {
	return file_spec_proto_rpc_proto_enumTypes[0].Descriptor()
}

func (Context_TargetType) Type() protoreflect.EnumType {
	return &file_spec_proto_rpc_proto_enumTypes[0]
}

func (x Context_TargetType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Context_TargetType.Descriptor instead.
func (Context_TargetType) EnumDescriptor() ([]byte, []int) {
	return file_spec_proto_rpc_proto_rawDescGZIP(), []int{0, 0}
}

type RPC_Type int32

const (
	RPC_UNKNOWN_DIR RPC_Type = 0
	RPC_REQUEST     RPC_Type = 1
	RPC_REPLY       RPC_Type = 2
)

// Enum value maps for RPC_Type.
var (
	RPC_Type_name = map[int32]string{
		0: "UNKNOWN_DIR",
		1: "REQUEST",
		2: "REPLY",
	}
	RPC_Type_value = map[string]int32{
		"UNKNOWN_DIR": 0,
		"REQUEST":     1,
		"REPLY":       2,
	}
)

func (x RPC_Type) Enum() *RPC_Type {
	p := new(RPC_Type)
	*p = x
	return p
}

func (x RPC_Type) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (RPC_Type) Descriptor() protoreflect.EnumDescriptor {
	return file_spec_proto_rpc_proto_enumTypes[1].Descriptor()
}

func (RPC_Type) Type() protoreflect.EnumType {
	return &file_spec_proto_rpc_proto_enumTypes[1]
}

func (x RPC_Type) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use RPC_Type.Descriptor instead.
func (RPC_Type) EnumDescriptor() ([]byte, []int) {
	return file_spec_proto_rpc_proto_rawDescGZIP(), []int{1, 0}
}

type RPC_Kind int32

const (
	RPC_UNKNOWN_TYPE RPC_Kind = 0
	// chord
	RPC_PING              RPC_Kind = 1
	RPC_NOTIFY            RPC_Kind = 2
	RPC_FIND_SUCCESSOR    RPC_Kind = 3
	RPC_GET_PREDECESSOR   RPC_Kind = 4
	RPC_KV                RPC_Kind = 5
	RPC_GET_SUCCESSORS    RPC_Kind = 6
	RPC_IDENTITY          RPC_Kind = 10
	RPC_MEMBERSHIP_CHANGE RPC_Kind = 11
	// tunnel
	RPC_CLIENT_REQUEST RPC_Kind = 50
)

// Enum value maps for RPC_Kind.
var (
	RPC_Kind_name = map[int32]string{
		0:  "UNKNOWN_TYPE",
		1:  "PING",
		2:  "NOTIFY",
		3:  "FIND_SUCCESSOR",
		4:  "GET_PREDECESSOR",
		5:  "KV",
		6:  "GET_SUCCESSORS",
		10: "IDENTITY",
		11: "MEMBERSHIP_CHANGE",
		50: "CLIENT_REQUEST",
	}
	RPC_Kind_value = map[string]int32{
		"UNKNOWN_TYPE":      0,
		"PING":              1,
		"NOTIFY":            2,
		"FIND_SUCCESSOR":    3,
		"GET_PREDECESSOR":   4,
		"KV":                5,
		"GET_SUCCESSORS":    6,
		"IDENTITY":          10,
		"MEMBERSHIP_CHANGE": 11,
		"CLIENT_REQUEST":    50,
	}
)

func (x RPC_Kind) Enum() *RPC_Kind {
	p := new(RPC_Kind)
	*p = x
	return p
}

func (x RPC_Kind) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (RPC_Kind) Descriptor() protoreflect.EnumDescriptor {
	return file_spec_proto_rpc_proto_enumTypes[2].Descriptor()
}

func (RPC_Kind) Type() protoreflect.EnumType {
	return &file_spec_proto_rpc_proto_enumTypes[2]
}

func (x RPC_Kind) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use RPC_Kind.Descriptor instead.
func (RPC_Kind) EnumDescriptor() ([]byte, []int) {
	return file_spec_proto_rpc_proto_rawDescGZIP(), []int{1, 1}
}

type Context struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	RequestTarget Context_TargetType `protobuf:"varint,1,opt,name=requestTarget,proto3,enum=protocol.Context_TargetType" json:"requestTarget,omitempty"`
}

func (x *Context) Reset() {
	*x = Context{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_rpc_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Context) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Context) ProtoMessage() {}

func (x *Context) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_rpc_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Context.ProtoReflect.Descriptor instead.
func (*Context) Descriptor() ([]byte, []int) {
	return file_spec_proto_rpc_proto_rawDescGZIP(), []int{0}
}

func (x *Context) GetRequestTarget() Context_TargetType {
	if x != nil {
		return x.RequestTarget
	}
	return Context_UNKNOWN
}

type RPC struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type     RPC_Type      `protobuf:"varint,1,opt,name=type,proto3,enum=protocol.RPC_Type" json:"type,omitempty"`
	ReqNum   uint64        `protobuf:"varint,2,opt,name=reqNum,proto3" json:"reqNum,omitempty"`
	Ring     uint64        `protobuf:"varint,3,opt,name=ring,proto3" json:"ring,omitempty"`
	Request  *RPC_Request  `protobuf:"bytes,10,opt,name=request,proto3" json:"request,omitempty"`
	Response *RPC_Response `protobuf:"bytes,11,opt,name=response,proto3" json:"response,omitempty"`
}

func (x *RPC) Reset() {
	*x = RPC{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_rpc_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RPC) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RPC) ProtoMessage() {}

func (x *RPC) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_rpc_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RPC.ProtoReflect.Descriptor instead.
func (*RPC) Descriptor() ([]byte, []int) {
	return file_spec_proto_rpc_proto_rawDescGZIP(), []int{1}
}

func (x *RPC) GetType() RPC_Type {
	if x != nil {
		return x.Type
	}
	return RPC_UNKNOWN_DIR
}

func (x *RPC) GetReqNum() uint64 {
	if x != nil {
		return x.ReqNum
	}
	return 0
}

func (x *RPC) GetRing() uint64 {
	if x != nil {
		return x.Ring
	}
	return 0
}

func (x *RPC) GetRequest() *RPC_Request {
	if x != nil {
		return x.Request
	}
	return nil
}

func (x *RPC) GetResponse() *RPC_Response {
	if x != nil {
		return x.Response
	}
	return nil
}

type RPC_Request struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Kind                  RPC_Kind                 `protobuf:"varint,1,opt,name=kind,proto3,enum=protocol.RPC_Kind" json:"kind,omitempty"`
	RequestContext        *Context                 `protobuf:"bytes,2,opt,name=requestContext,proto3" json:"requestContext,omitempty"`
	IdentityRequest       *IdentityRequest         `protobuf:"bytes,5,opt,name=identityRequest,proto3" json:"identityRequest,omitempty"`
	PingRequest           *PingRequest             `protobuf:"bytes,10,opt,name=pingRequest,proto3" json:"pingRequest,omitempty"`
	NotifyRequest         *NotifyRequest           `protobuf:"bytes,11,opt,name=notifyRequest,proto3" json:"notifyRequest,omitempty"`
	FindSuccessorRequest  *FindSuccessorRequest    `protobuf:"bytes,20,opt,name=findSuccessorRequest,proto3" json:"findSuccessorRequest,omitempty"`
	GetPredecessorRequest *GetPredecessorRequest   `protobuf:"bytes,21,opt,name=getPredecessorRequest,proto3" json:"getPredecessorRequest,omitempty"`
	GetSuccessorsRequest  *GetSuccessorsRequest    `protobuf:"bytes,30,opt,name=getSuccessorsRequest,proto3" json:"getSuccessorsRequest,omitempty"`
	KvRequest             *KVRequest               `protobuf:"bytes,41,opt,name=kvRequest,proto3" json:"kvRequest,omitempty"`
	MembershipRequest     *MembershipChangeRequest `protobuf:"bytes,50,opt,name=membershipRequest,proto3" json:"membershipRequest,omitempty"`
	ClientRequest         *ClientRequest           `protobuf:"bytes,60,opt,name=clientRequest,proto3" json:"clientRequest,omitempty"`
}

func (x *RPC_Request) Reset() {
	*x = RPC_Request{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_rpc_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RPC_Request) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RPC_Request) ProtoMessage() {}

func (x *RPC_Request) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_rpc_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RPC_Request.ProtoReflect.Descriptor instead.
func (*RPC_Request) Descriptor() ([]byte, []int) {
	return file_spec_proto_rpc_proto_rawDescGZIP(), []int{1, 0}
}

func (x *RPC_Request) GetKind() RPC_Kind {
	if x != nil {
		return x.Kind
	}
	return RPC_UNKNOWN_TYPE
}

func (x *RPC_Request) GetRequestContext() *Context {
	if x != nil {
		return x.RequestContext
	}
	return nil
}

func (x *RPC_Request) GetIdentityRequest() *IdentityRequest {
	if x != nil {
		return x.IdentityRequest
	}
	return nil
}

func (x *RPC_Request) GetPingRequest() *PingRequest {
	if x != nil {
		return x.PingRequest
	}
	return nil
}

func (x *RPC_Request) GetNotifyRequest() *NotifyRequest {
	if x != nil {
		return x.NotifyRequest
	}
	return nil
}

func (x *RPC_Request) GetFindSuccessorRequest() *FindSuccessorRequest {
	if x != nil {
		return x.FindSuccessorRequest
	}
	return nil
}

func (x *RPC_Request) GetGetPredecessorRequest() *GetPredecessorRequest {
	if x != nil {
		return x.GetPredecessorRequest
	}
	return nil
}

func (x *RPC_Request) GetGetSuccessorsRequest() *GetSuccessorsRequest {
	if x != nil {
		return x.GetSuccessorsRequest
	}
	return nil
}

func (x *RPC_Request) GetKvRequest() *KVRequest {
	if x != nil {
		return x.KvRequest
	}
	return nil
}

func (x *RPC_Request) GetMembershipRequest() *MembershipChangeRequest {
	if x != nil {
		return x.MembershipRequest
	}
	return nil
}

func (x *RPC_Request) GetClientRequest() *ClientRequest {
	if x != nil {
		return x.ClientRequest
	}
	return nil
}

type RPC_Response struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Error                  []byte                    `protobuf:"bytes,1,opt,name=error,proto3" json:"error,omitempty"`
	IdentityResponse       *IdentityResponse         `protobuf:"bytes,5,opt,name=identityResponse,proto3" json:"identityResponse,omitempty"`
	PingResponse           *PingResponse             `protobuf:"bytes,10,opt,name=pingResponse,proto3" json:"pingResponse,omitempty"`
	NotifyResponse         *NotifyResponse           `protobuf:"bytes,11,opt,name=notifyResponse,proto3" json:"notifyResponse,omitempty"`
	FindSuccessorResponse  *FindSuccessorResponse    `protobuf:"bytes,20,opt,name=findSuccessorResponse,proto3" json:"findSuccessorResponse,omitempty"`
	GetPredecessorResponse *GetPredecessorResponse   `protobuf:"bytes,21,opt,name=getPredecessorResponse,proto3" json:"getPredecessorResponse,omitempty"`
	GetSuccessorsResponse  *GetSuccessorsResponse    `protobuf:"bytes,30,opt,name=getSuccessorsResponse,proto3" json:"getSuccessorsResponse,omitempty"`
	KvResponse             *KVResponse               `protobuf:"bytes,41,opt,name=kvResponse,proto3" json:"kvResponse,omitempty"`
	MembershipResponse     *MembershipChangeResponse `protobuf:"bytes,50,opt,name=membershipResponse,proto3" json:"membershipResponse,omitempty"`
	ClientResponse         *ClientResponse           `protobuf:"bytes,60,opt,name=clientResponse,proto3" json:"clientResponse,omitempty"`
}

func (x *RPC_Response) Reset() {
	*x = RPC_Response{}
	if protoimpl.UnsafeEnabled {
		mi := &file_spec_proto_rpc_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RPC_Response) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RPC_Response) ProtoMessage() {}

func (x *RPC_Response) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_rpc_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RPC_Response.ProtoReflect.Descriptor instead.
func (*RPC_Response) Descriptor() ([]byte, []int) {
	return file_spec_proto_rpc_proto_rawDescGZIP(), []int{1, 1}
}

func (x *RPC_Response) GetError() []byte {
	if x != nil {
		return x.Error
	}
	return nil
}

func (x *RPC_Response) GetIdentityResponse() *IdentityResponse {
	if x != nil {
		return x.IdentityResponse
	}
	return nil
}

func (x *RPC_Response) GetPingResponse() *PingResponse {
	if x != nil {
		return x.PingResponse
	}
	return nil
}

func (x *RPC_Response) GetNotifyResponse() *NotifyResponse {
	if x != nil {
		return x.NotifyResponse
	}
	return nil
}

func (x *RPC_Response) GetFindSuccessorResponse() *FindSuccessorResponse {
	if x != nil {
		return x.FindSuccessorResponse
	}
	return nil
}

func (x *RPC_Response) GetGetPredecessorResponse() *GetPredecessorResponse {
	if x != nil {
		return x.GetPredecessorResponse
	}
	return nil
}

func (x *RPC_Response) GetGetSuccessorsResponse() *GetSuccessorsResponse {
	if x != nil {
		return x.GetSuccessorsResponse
	}
	return nil
}

func (x *RPC_Response) GetKvResponse() *KVResponse {
	if x != nil {
		return x.KvResponse
	}
	return nil
}

func (x *RPC_Response) GetMembershipResponse() *MembershipChangeResponse {
	if x != nil {
		return x.MembershipResponse
	}
	return nil
}

func (x *RPC_Response) GetClientResponse() *ClientResponse {
	if x != nil {
		return x.ClientResponse
	}
	return nil
}

var File_spec_proto_rpc_proto protoreflect.FileDescriptor

var file_spec_proto_rpc_proto_rawDesc = []byte{
	0x0a, 0x14, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x72, 0x70, 0x63,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c,
	0x1a, 0x16, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x63, 0x68, 0x6f,
	0x72, 0x64, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x13, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x6b, 0x76, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x14, 0x73,
	0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x74, 0x75, 0x6e, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x22, 0x7c, 0x0a, 0x07, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x12, 0x42,
	0x0a, 0x0d, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x1c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c,
	0x2e, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x2e, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x54,
	0x79, 0x70, 0x65, 0x52, 0x0d, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x54, 0x61, 0x72, 0x67,
	0x65, 0x74, 0x22, 0x2d, 0x0a, 0x0a, 0x54, 0x61, 0x72, 0x67, 0x65, 0x74, 0x54, 0x79, 0x70, 0x65,
	0x12, 0x0b, 0x0a, 0x07, 0x55, 0x4e, 0x4b, 0x4e, 0x4f, 0x57, 0x4e, 0x10, 0x00, 0x12, 0x12, 0x0a,
	0x0e, 0x4b, 0x56, 0x5f, 0x52, 0x45, 0x50, 0x4c, 0x49, 0x43, 0x41, 0x54, 0x49, 0x4f, 0x4e, 0x10,
	0x01, 0x22, 0xc9, 0x0e, 0x0a, 0x03, 0x52, 0x50, 0x43, 0x12, 0x26, 0x0a, 0x04, 0x74, 0x79, 0x70,
	0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x12, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63,
	0x6f, 0x6c, 0x2e, 0x52, 0x50, 0x43, 0x2e, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74, 0x79, 0x70,
	0x65, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x65, 0x71, 0x4e, 0x75, 0x6d, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x04, 0x52, 0x06, 0x72, 0x65, 0x71, 0x4e, 0x75, 0x6d, 0x12, 0x12, 0x0a, 0x04, 0x72, 0x69, 0x6e,
	0x67, 0x18, 0x03, 0x20, 0x01, 0x28, 0x04, 0x52, 0x04, 0x72, 0x69, 0x6e, 0x67, 0x12, 0x2f, 0x0a,
	0x07, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x15,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x52, 0x50, 0x43, 0x2e, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x52, 0x07, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x32,
	0x0a, 0x08, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x16, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x52, 0x50, 0x43, 0x2e,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x08, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x1a, 0xeb, 0x05, 0x0a, 0x07, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x26,
	0x0a, 0x04, 0x6b, 0x69, 0x6e, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x12, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x52, 0x50, 0x43, 0x2e, 0x4b, 0x69, 0x6e, 0x64,
	0x52, 0x04, 0x6b, 0x69, 0x6e, 0x64, 0x12, 0x39, 0x0a, 0x0e, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x11,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x78,
	0x74, 0x52, 0x0e, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x78,
	0x74, 0x12, 0x43, 0x0a, 0x0f, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x49, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x52, 0x0f, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x37, 0x0a, 0x0b, 0x70, 0x69, 0x6e, 0x67, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x15, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x52, 0x0b, 0x70, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12,
	0x3d, 0x0a, 0x0d, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x18, 0x0b, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f,
	0x6c, 0x2e, 0x4e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x52,
	0x0d, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x52,
	0x0a, 0x14, 0x66, 0x69, 0x6e, 0x64, 0x53, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x18, 0x14, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x46, 0x69, 0x6e, 0x64, 0x53, 0x75, 0x63, 0x63,
	0x65, 0x73, 0x73, 0x6f, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x52, 0x14, 0x66, 0x69,
	0x6e, 0x64, 0x53, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x12, 0x55, 0x0a, 0x15, 0x67, 0x65, 0x74, 0x50, 0x72, 0x65, 0x64, 0x65, 0x63, 0x65,
	0x73, 0x73, 0x6f, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x18, 0x15, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x1f, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x47, 0x65, 0x74,
	0x50, 0x72, 0x65, 0x64, 0x65, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x52, 0x15, 0x67, 0x65, 0x74, 0x50, 0x72, 0x65, 0x64, 0x65, 0x63, 0x65, 0x73, 0x73,
	0x6f, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x52, 0x0a, 0x14, 0x67, 0x65, 0x74,
	0x53, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x18, 0x1e, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63,
	0x6f, 0x6c, 0x2e, 0x47, 0x65, 0x74, 0x53, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x73,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x52, 0x14, 0x67, 0x65, 0x74, 0x53, 0x75, 0x63, 0x63,
	0x65, 0x73, 0x73, 0x6f, 0x72, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x31, 0x0a,
	0x09, 0x6b, 0x76, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x18, 0x29, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x13, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4b, 0x56, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x52, 0x09, 0x6b, 0x76, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x12, 0x4f, 0x0a, 0x11, 0x6d, 0x65, 0x6d, 0x62, 0x65, 0x72, 0x73, 0x68, 0x69, 0x70, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x18, 0x32, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x21, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4d, 0x65, 0x6d, 0x62, 0x65, 0x72, 0x73, 0x68, 0x69,
	0x70, 0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x52, 0x11,
	0x6d, 0x65, 0x6d, 0x62, 0x65, 0x72, 0x73, 0x68, 0x69, 0x70, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x3d, 0x0a, 0x0d, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x18, 0x3c, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x63, 0x6f, 0x6c, 0x2e, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x52, 0x0d, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0xba, 0x05, 0x0a, 0x08, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x14, 0x0a,
	0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x05, 0x65, 0x72,
	0x72, 0x6f, 0x72, 0x12, 0x46, 0x0a, 0x10, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x49, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74,
	0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x10, 0x69, 0x64, 0x65, 0x6e, 0x74,
	0x69, 0x74, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3a, 0x0a, 0x0c, 0x70,
	0x69, 0x6e, 0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x0a, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x16, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x50, 0x69, 0x6e,
	0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x0c, 0x70, 0x69, 0x6e, 0x67, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x40, 0x0a, 0x0e, 0x6e, 0x6f, 0x74, 0x69, 0x66,
	0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x18, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4e, 0x6f, 0x74, 0x69, 0x66,
	0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x0e, 0x6e, 0x6f, 0x74, 0x69, 0x66,
	0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x55, 0x0a, 0x15, 0x66, 0x69, 0x6e,
	0x64, 0x53, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x18, 0x14, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1f, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x63, 0x6f, 0x6c, 0x2e, 0x46, 0x69, 0x6e, 0x64, 0x53, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f,
	0x72, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x15, 0x66, 0x69, 0x6e, 0x64, 0x53,
	0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x58, 0x0a, 0x16, 0x67, 0x65, 0x74, 0x50, 0x72, 0x65, 0x64, 0x65, 0x63, 0x65, 0x73, 0x73,
	0x6f, 0x72, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x15, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x20, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x47, 0x65, 0x74, 0x50,
	0x72, 0x65, 0x64, 0x65, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x52, 0x16, 0x67, 0x65, 0x74, 0x50, 0x72, 0x65, 0x64, 0x65, 0x63, 0x65, 0x73, 0x73,
	0x6f, 0x72, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x55, 0x0a, 0x15, 0x67, 0x65,
	0x74, 0x53, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x18, 0x1e, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1f, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x47, 0x65, 0x74, 0x53, 0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f,
	0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x15, 0x67, 0x65, 0x74, 0x53,
	0x75, 0x63, 0x63, 0x65, 0x73, 0x73, 0x6f, 0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x12, 0x34, 0x0a, 0x0a, 0x6b, 0x76, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18,
	0x29, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c,
	0x2e, 0x4b, 0x56, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x0a, 0x6b, 0x76, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x52, 0x0a, 0x12, 0x6d, 0x65, 0x6d, 0x62, 0x65,
	0x72, 0x73, 0x68, 0x69, 0x70, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x32, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x22, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x4d,
	0x65, 0x6d, 0x62, 0x65, 0x72, 0x73, 0x68, 0x69, 0x70, 0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x12, 0x6d, 0x65, 0x6d, 0x62, 0x65, 0x72, 0x73,
	0x68, 0x69, 0x70, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x40, 0x0a, 0x0e, 0x63,
	0x6c, 0x69, 0x65, 0x6e, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x3c, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x18, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x43,
	0x6c, 0x69, 0x65, 0x6e, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x0e, 0x63,
	0x6c, 0x69, 0x65, 0x6e, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x2f, 0x0a,
	0x04, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0f, 0x0a, 0x0b, 0x55, 0x4e, 0x4b, 0x4e, 0x4f, 0x57, 0x4e,
	0x5f, 0x44, 0x49, 0x52, 0x10, 0x00, 0x12, 0x0b, 0x0a, 0x07, 0x52, 0x45, 0x51, 0x55, 0x45, 0x53,
	0x54, 0x10, 0x01, 0x12, 0x09, 0x0a, 0x05, 0x52, 0x45, 0x50, 0x4c, 0x59, 0x10, 0x02, 0x22, 0xac,
	0x01, 0x0a, 0x04, 0x4b, 0x69, 0x6e, 0x64, 0x12, 0x10, 0x0a, 0x0c, 0x55, 0x4e, 0x4b, 0x4e, 0x4f,
	0x57, 0x4e, 0x5f, 0x54, 0x59, 0x50, 0x45, 0x10, 0x00, 0x12, 0x08, 0x0a, 0x04, 0x50, 0x49, 0x4e,
	0x47, 0x10, 0x01, 0x12, 0x0a, 0x0a, 0x06, 0x4e, 0x4f, 0x54, 0x49, 0x46, 0x59, 0x10, 0x02, 0x12,
	0x12, 0x0a, 0x0e, 0x46, 0x49, 0x4e, 0x44, 0x5f, 0x53, 0x55, 0x43, 0x43, 0x45, 0x53, 0x53, 0x4f,
	0x52, 0x10, 0x03, 0x12, 0x13, 0x0a, 0x0f, 0x47, 0x45, 0x54, 0x5f, 0x50, 0x52, 0x45, 0x44, 0x45,
	0x43, 0x45, 0x53, 0x53, 0x4f, 0x52, 0x10, 0x04, 0x12, 0x06, 0x0a, 0x02, 0x4b, 0x56, 0x10, 0x05,
	0x12, 0x12, 0x0a, 0x0e, 0x47, 0x45, 0x54, 0x5f, 0x53, 0x55, 0x43, 0x43, 0x45, 0x53, 0x53, 0x4f,
	0x52, 0x53, 0x10, 0x06, 0x12, 0x0c, 0x0a, 0x08, 0x49, 0x44, 0x45, 0x4e, 0x54, 0x49, 0x54, 0x59,
	0x10, 0x0a, 0x12, 0x15, 0x0a, 0x11, 0x4d, 0x45, 0x4d, 0x42, 0x45, 0x52, 0x53, 0x48, 0x49, 0x50,
	0x5f, 0x43, 0x48, 0x41, 0x4e, 0x47, 0x45, 0x10, 0x0b, 0x12, 0x12, 0x0a, 0x0e, 0x43, 0x4c, 0x49,
	0x45, 0x4e, 0x54, 0x5f, 0x52, 0x45, 0x51, 0x55, 0x45, 0x53, 0x54, 0x10, 0x32, 0x42, 0x25, 0x48,
	0x01, 0x5a, 0x21, 0x6b, 0x6f, 0x6e, 0x2e, 0x6e, 0x65, 0x63, 0x74, 0x2e, 0x73, 0x68, 0x2f, 0x73,
	0x70, 0x65, 0x63, 0x74, 0x65, 0x72, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x63, 0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_spec_proto_rpc_proto_rawDescOnce sync.Once
	file_spec_proto_rpc_proto_rawDescData = file_spec_proto_rpc_proto_rawDesc
)

func file_spec_proto_rpc_proto_rawDescGZIP() []byte {
	file_spec_proto_rpc_proto_rawDescOnce.Do(func() {
		file_spec_proto_rpc_proto_rawDescData = protoimpl.X.CompressGZIP(file_spec_proto_rpc_proto_rawDescData)
	})
	return file_spec_proto_rpc_proto_rawDescData
}

var file_spec_proto_rpc_proto_enumTypes = make([]protoimpl.EnumInfo, 3)
var file_spec_proto_rpc_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_spec_proto_rpc_proto_goTypes = []interface{}{
	(Context_TargetType)(0),          // 0: protocol.Context.TargetType
	(RPC_Type)(0),                    // 1: protocol.RPC.Type
	(RPC_Kind)(0),                    // 2: protocol.RPC.Kind
	(*Context)(nil),                  // 3: protocol.Context
	(*RPC)(nil),                      // 4: protocol.RPC
	(*RPC_Request)(nil),              // 5: protocol.RPC.Request
	(*RPC_Response)(nil),             // 6: protocol.RPC.Response
	(*IdentityRequest)(nil),          // 7: protocol.IdentityRequest
	(*PingRequest)(nil),              // 8: protocol.PingRequest
	(*NotifyRequest)(nil),            // 9: protocol.NotifyRequest
	(*FindSuccessorRequest)(nil),     // 10: protocol.FindSuccessorRequest
	(*GetPredecessorRequest)(nil),    // 11: protocol.GetPredecessorRequest
	(*GetSuccessorsRequest)(nil),     // 12: protocol.GetSuccessorsRequest
	(*KVRequest)(nil),                // 13: protocol.KVRequest
	(*MembershipChangeRequest)(nil),  // 14: protocol.MembershipChangeRequest
	(*ClientRequest)(nil),            // 15: protocol.ClientRequest
	(*IdentityResponse)(nil),         // 16: protocol.IdentityResponse
	(*PingResponse)(nil),             // 17: protocol.PingResponse
	(*NotifyResponse)(nil),           // 18: protocol.NotifyResponse
	(*FindSuccessorResponse)(nil),    // 19: protocol.FindSuccessorResponse
	(*GetPredecessorResponse)(nil),   // 20: protocol.GetPredecessorResponse
	(*GetSuccessorsResponse)(nil),    // 21: protocol.GetSuccessorsResponse
	(*KVResponse)(nil),               // 22: protocol.KVResponse
	(*MembershipChangeResponse)(nil), // 23: protocol.MembershipChangeResponse
	(*ClientResponse)(nil),           // 24: protocol.ClientResponse
}
var file_spec_proto_rpc_proto_depIdxs = []int32{
	0,  // 0: protocol.Context.requestTarget:type_name -> protocol.Context.TargetType
	1,  // 1: protocol.RPC.type:type_name -> protocol.RPC.Type
	5,  // 2: protocol.RPC.request:type_name -> protocol.RPC.Request
	6,  // 3: protocol.RPC.response:type_name -> protocol.RPC.Response
	2,  // 4: protocol.RPC.Request.kind:type_name -> protocol.RPC.Kind
	3,  // 5: protocol.RPC.Request.requestContext:type_name -> protocol.Context
	7,  // 6: protocol.RPC.Request.identityRequest:type_name -> protocol.IdentityRequest
	8,  // 7: protocol.RPC.Request.pingRequest:type_name -> protocol.PingRequest
	9,  // 8: protocol.RPC.Request.notifyRequest:type_name -> protocol.NotifyRequest
	10, // 9: protocol.RPC.Request.findSuccessorRequest:type_name -> protocol.FindSuccessorRequest
	11, // 10: protocol.RPC.Request.getPredecessorRequest:type_name -> protocol.GetPredecessorRequest
	12, // 11: protocol.RPC.Request.getSuccessorsRequest:type_name -> protocol.GetSuccessorsRequest
	13, // 12: protocol.RPC.Request.kvRequest:type_name -> protocol.KVRequest
	14, // 13: protocol.RPC.Request.membershipRequest:type_name -> protocol.MembershipChangeRequest
	15, // 14: protocol.RPC.Request.clientRequest:type_name -> protocol.ClientRequest
	16, // 15: protocol.RPC.Response.identityResponse:type_name -> protocol.IdentityResponse
	17, // 16: protocol.RPC.Response.pingResponse:type_name -> protocol.PingResponse
	18, // 17: protocol.RPC.Response.notifyResponse:type_name -> protocol.NotifyResponse
	19, // 18: protocol.RPC.Response.findSuccessorResponse:type_name -> protocol.FindSuccessorResponse
	20, // 19: protocol.RPC.Response.getPredecessorResponse:type_name -> protocol.GetPredecessorResponse
	21, // 20: protocol.RPC.Response.getSuccessorsResponse:type_name -> protocol.GetSuccessorsResponse
	22, // 21: protocol.RPC.Response.kvResponse:type_name -> protocol.KVResponse
	23, // 22: protocol.RPC.Response.membershipResponse:type_name -> protocol.MembershipChangeResponse
	24, // 23: protocol.RPC.Response.clientResponse:type_name -> protocol.ClientResponse
	24, // [24:24] is the sub-list for method output_type
	24, // [24:24] is the sub-list for method input_type
	24, // [24:24] is the sub-list for extension type_name
	24, // [24:24] is the sub-list for extension extendee
	0,  // [0:24] is the sub-list for field type_name
}

func init() { file_spec_proto_rpc_proto_init() }
func file_spec_proto_rpc_proto_init() {
	if File_spec_proto_rpc_proto != nil {
		return
	}
	file_spec_proto_chord_proto_init()
	file_spec_proto_kv_proto_init()
	file_spec_proto_tun_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_spec_proto_rpc_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Context); i {
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
		file_spec_proto_rpc_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RPC); i {
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
		file_spec_proto_rpc_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RPC_Request); i {
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
		file_spec_proto_rpc_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RPC_Response); i {
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
			RawDescriptor: file_spec_proto_rpc_proto_rawDesc,
			NumEnums:      3,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_spec_proto_rpc_proto_goTypes,
		DependencyIndexes: file_spec_proto_rpc_proto_depIdxs,
		EnumInfos:         file_spec_proto_rpc_proto_enumTypes,
		MessageInfos:      file_spec_proto_rpc_proto_msgTypes,
	}.Build()
	File_spec_proto_rpc_proto = out.File
	file_spec_proto_rpc_proto_rawDesc = nil
	file_spec_proto_rpc_proto_goTypes = nil
	file_spec_proto_rpc_proto_depIdxs = nil
}
