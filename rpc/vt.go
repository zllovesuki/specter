package rpc

type VTMarshaler interface {
	MarshalVT() (dAtA []byte, err error)
	UnmarshalVT(dAtA []byte) error
}
