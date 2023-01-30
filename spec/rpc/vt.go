package rpc

type VTMarshaler interface {
	MarshalToSizedBufferVT(dAtA []byte) (n int, err error)
	MarshalVT() (dAtA []byte, err error)
	UnmarshalVT(dAtA []byte) error
	SizeVT() int
}
