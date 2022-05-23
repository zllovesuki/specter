package server

import (
	"context"
	"net"

	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/rpc"
	"github.com/zllovesuki/specter/spec/transport"

	"github.com/stretchr/testify/mock"
)

type mockTransport struct {
	mock.Mock
}

var _ transport.Transport = (*mockTransport)(nil)

func (t *mockTransport) Identity() *protocol.Node {
	args := t.Called()
	v := args.Get(0)
	if v == nil {
		return nil
	}
	return v.(*protocol.Node)
}

func (t *mockTransport) DialRPC(ctx context.Context, peer *protocol.Node, hs rpc.RPCHandshakeFunc) (rpc.RPC, error) {
	args := t.Called(ctx, peer, hs)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(rpc.RPC), e
}

func (t *mockTransport) DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error) {
	args := t.Called(ctx, peer)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(net.Conn), e
}

func (t *mockTransport) RPC() <-chan *transport.StreamDelegate {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *transport.StreamDelegate)
}

func (t *mockTransport) Direct() <-chan *transport.StreamDelegate {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *transport.StreamDelegate)
}

func (t *mockTransport) SupportDatagram() bool {
	args := t.Called()
	v := args.Bool(0)
	return v
}

func (t *mockTransport) ReceiveDatagram() <-chan *transport.DatagramDelegate {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *transport.DatagramDelegate)
}

func (t *mockTransport) SendDatagram(n *protocol.Node, b []byte) error {
	args := t.Called(n, b)
	e := args.Error(0)
	return e
}

func (t *mockTransport) TransportEstablished() <-chan *protocol.Node {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *protocol.Node)
}

func (t *mockTransport) TransportDestroyed() <-chan *protocol.Node {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *protocol.Node)
}

func (t *mockTransport) Accept(ctx context.Context) error {
	args := t.Called(ctx)
	e := args.Error(0)
	return e
}

func (t *mockTransport) Stop() {
	t.Called()
}
