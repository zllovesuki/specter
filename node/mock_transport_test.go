package node

import (
	"context"
	"net"

	"specter/spec/protocol"
	"specter/spec/rpc"
)

type mockTransport struct{}

func (*mockTransport) DialRPC(ctx context.Context, peer *protocol.Node, hs rpc.RPCHandshakeFunc) (rpc.RPC, error) {
	panic("not implemented")
}

func (*mockTransport) DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error) {
	panic("not implemented")
}

func (*mockTransport) RPC() <-chan net.Conn {
	panic("not implemented")
}

func (*mockTransport) Direct() <-chan net.Conn {
	panic("not implemented")
}

func (*mockTransport) Accept(ctx context.Context, identity *protocol.Node) error {
	panic("not implemented")
}

func (*mockTransport) Stop() {
	panic("not implemented")
}
