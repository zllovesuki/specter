// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.4
// 	protoc        v5.29.3
// source: spec/proto/pki.proto

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

type CertificateRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Proof         *ProofOfWork           `protobuf:"bytes,1,opt,name=proof,proto3" json:"proof,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CertificateRequest) Reset() {
	*x = CertificateRequest{}
	mi := &file_spec_proto_pki_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CertificateRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CertificateRequest) ProtoMessage() {}

func (x *CertificateRequest) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_pki_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CertificateRequest.ProtoReflect.Descriptor instead.
func (*CertificateRequest) Descriptor() ([]byte, []int) {
	return file_spec_proto_pki_proto_rawDescGZIP(), []int{0}
}

func (x *CertificateRequest) GetProof() *ProofOfWork {
	if x != nil {
		return x.Proof
	}
	return nil
}

type CertificateResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	CertDer       []byte                 `protobuf:"bytes,1,opt,name=cert_der,json=certDer,proto3" json:"cert_der,omitempty"`
	CertPem       []byte                 `protobuf:"bytes,2,opt,name=cert_pem,json=certPem,proto3" json:"cert_pem,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CertificateResponse) Reset() {
	*x = CertificateResponse{}
	mi := &file_spec_proto_pki_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CertificateResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CertificateResponse) ProtoMessage() {}

func (x *CertificateResponse) ProtoReflect() protoreflect.Message {
	mi := &file_spec_proto_pki_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CertificateResponse.ProtoReflect.Descriptor instead.
func (*CertificateResponse) Descriptor() ([]byte, []int) {
	return file_spec_proto_pki_proto_rawDescGZIP(), []int{1}
}

func (x *CertificateResponse) GetCertDer() []byte {
	if x != nil {
		return x.CertDer
	}
	return nil
}

func (x *CertificateResponse) GetCertPem() []byte {
	if x != nil {
		return x.CertPem
	}
	return nil
}

var File_spec_proto_pki_proto protoreflect.FileDescriptor

var file_spec_proto_pki_proto_rawDesc = string([]byte{
	0x0a, 0x14, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x70, 0x6b, 0x69,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c,
	0x1a, 0x14, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x70, 0x6f, 0x77,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x41, 0x0a, 0x12, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66,
	0x69, 0x63, 0x61, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x2b, 0x0a, 0x05,
	0x70, 0x72, 0x6f, 0x6f, 0x66, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x15, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x50, 0x72, 0x6f, 0x6f, 0x66, 0x4f, 0x66, 0x57, 0x6f,
	0x72, 0x6b, 0x52, 0x05, 0x70, 0x72, 0x6f, 0x6f, 0x66, 0x22, 0x4b, 0x0a, 0x13, 0x43, 0x65, 0x72,
	0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x19, 0x0a, 0x08, 0x63, 0x65, 0x72, 0x74, 0x5f, 0x64, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0c, 0x52, 0x07, 0x63, 0x65, 0x72, 0x74, 0x44, 0x65, 0x72, 0x12, 0x19, 0x0a, 0x08, 0x63,
	0x65, 0x72, 0x74, 0x5f, 0x70, 0x65, 0x6d, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x63,
	0x65, 0x72, 0x74, 0x50, 0x65, 0x6d, 0x32, 0x5f, 0x0a, 0x0a, 0x50, 0x4b, 0x49, 0x53, 0x65, 0x72,
	0x76, 0x69, 0x63, 0x65, 0x12, 0x51, 0x0a, 0x12, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x43,
	0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x12, 0x1c, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x2e, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1d, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x63, 0x6f, 0x6c, 0x2e, 0x43, 0x65, 0x72, 0x74, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x65, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0x2b, 0x48, 0x01, 0x5a, 0x27, 0x67, 0x6f, 0x2e,
	0x6d, 0x69, 0x72, 0x61, 0x67, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x2e, 0x63, 0x6f, 0x2f, 0x73,
	0x70, 0x65, 0x63, 0x74, 0x65, 0x72, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x63, 0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_spec_proto_pki_proto_rawDescOnce sync.Once
	file_spec_proto_pki_proto_rawDescData []byte
)

func file_spec_proto_pki_proto_rawDescGZIP() []byte {
	file_spec_proto_pki_proto_rawDescOnce.Do(func() {
		file_spec_proto_pki_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_spec_proto_pki_proto_rawDesc), len(file_spec_proto_pki_proto_rawDesc)))
	})
	return file_spec_proto_pki_proto_rawDescData
}

var file_spec_proto_pki_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_spec_proto_pki_proto_goTypes = []any{
	(*CertificateRequest)(nil),  // 0: protocol.CertificateRequest
	(*CertificateResponse)(nil), // 1: protocol.CertificateResponse
	(*ProofOfWork)(nil),         // 2: protocol.ProofOfWork
}
var file_spec_proto_pki_proto_depIdxs = []int32{
	2, // 0: protocol.CertificateRequest.proof:type_name -> protocol.ProofOfWork
	0, // 1: protocol.PKIService.RequestCertificate:input_type -> protocol.CertificateRequest
	1, // 2: protocol.PKIService.RequestCertificate:output_type -> protocol.CertificateResponse
	2, // [2:3] is the sub-list for method output_type
	1, // [1:2] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_spec_proto_pki_proto_init() }
func file_spec_proto_pki_proto_init() {
	if File_spec_proto_pki_proto != nil {
		return
	}
	file_spec_proto_pow_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_spec_proto_pki_proto_rawDesc), len(file_spec_proto_pki_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_spec_proto_pki_proto_goTypes,
		DependencyIndexes: file_spec_proto_pki_proto_depIdxs,
		MessageInfos:      file_spec_proto_pki_proto_msgTypes,
	}.Build()
	File_spec_proto_pki_proto = out.File
	file_spec_proto_pki_proto_goTypes = nil
	file_spec_proto_pki_proto_depIdxs = nil
}
