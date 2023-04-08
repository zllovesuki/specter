package protocol

import "go.uber.org/zap/zapcore"

var _ zapcore.ObjectMarshaler = (*Node)(nil)

func (n *Node) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if n.GetAddress() != "" {
		enc.AddString("address", n.GetAddress())
	}
	if n.GetId() != 0 {
		enc.AddUint64("id", n.GetId())
	}
	if n.GetRendezvous() {
		enc.AddBool("rendezvous", true)
	}
	return nil
}
