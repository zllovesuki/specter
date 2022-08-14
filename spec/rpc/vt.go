package rpc

type VTMarshaler interface {
	MarshalToSizedBufferVT(dAtA []byte) (n int, err error)
	UnmarshalVT(dAtA []byte) error
	SizeVT() int
}
