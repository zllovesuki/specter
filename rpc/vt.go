package rpc

type VTMarshaler interface {
	MarshalVT() (dAtA []byte, err error)
	MarshalToSizedBufferVT(dAtA []byte) (n int, err error)
	UnmarshalVT(dAtA []byte) error
	SizeVT() int
}
