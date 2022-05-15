package transport

import (
	"context"
	"net"

	"specter/spec/protocol"
	"specter/spec/rpc"
)

type Transport interface {
	DialRPC(ctx context.Context, peer *protocol.Node, hs rpc.RPCHandshakeFunc) (rpc.RPC, error)
	DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error)

	RPC() <-chan net.Conn
	Direct() <-chan net.Conn

	Accept(ctx context.Context, identity *protocol.Node) error

	Stop()
}
