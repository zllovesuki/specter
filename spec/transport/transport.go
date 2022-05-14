package transport

import (
	"context"
	"net"

	"specter/spec"
	"specter/spec/protocol"
)

type Stream struct {
	Connection net.Conn
	Remote     net.Addr
}

type Transport interface {
	DialRPC(ctx context.Context, peer *protocol.Node, hs spec.RPCHandshakeFunc) (spec.RPC, error)
	DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error)

	RPC() <-chan Stream
	Direct() <-chan Stream

	Accept(ctx context.Context, identity *protocol.Node) error
}
