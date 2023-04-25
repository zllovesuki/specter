package protocol

type vtMarshaler interface {
	MarshalVT() (dAtA []byte, err error)
	UnmarshalVT(dAtA []byte) error
}
