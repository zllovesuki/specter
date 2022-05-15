package transport

import (
	"context"
	"net"

	"specter/spec/protocol"
	"specter/spec/rpc"
)

type Delegate struct {
	Connection net.Conn
	Identity   *protocol.Node
}

type Transport interface {
	Identity() *protocol.Node

	DialRPC(ctx context.Context, peer *protocol.Node, hs rpc.RPCHandshakeFunc) (rpc.RPC, error)
	DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error)

	RPC() <-chan *Delegate
	Direct() <-chan *Delegate

	Accept(ctx context.Context) error

	Stop()
}
