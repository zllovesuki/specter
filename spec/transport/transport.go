package transport

import (
	"context"
	"net"

	"specter/spec"
	"specter/spec/protocol"
)

type Transport interface {
	DialRPC(ctx context.Context, peer *protocol.Node, hs spec.RPCHandshakeFunc) (spec.RPC, error)
	DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error)

	RPC() <-chan net.Conn
	Direct() <-chan net.Conn

	Accept(ctx context.Context, identity *protocol.Node) error
}
