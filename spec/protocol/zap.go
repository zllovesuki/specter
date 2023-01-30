package protocol

import "go.uber.org/zap/zapcore"

var _ zapcore.ObjectMarshaler = (*Node)(nil)

func (n *Node) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if n.GetAddress() != "" {
		enc.AddString("address", n.GetAddress())
	}
	enc.AddUint64("id", n.GetId())
	return nil
}
