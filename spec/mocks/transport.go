package mocks

import (
	"context"
	"net"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"

	"github.com/stretchr/testify/mock"
)

type Transport struct {
	mock.Mock
}

var _ transport.Transport = (*Transport)(nil)

func (t *Transport) Identity() *protocol.Node {
	args := t.Called()
	v := args.Get(0)
	if v == nil {
		return nil
	}
	return v.(*protocol.Node)
}

func (t *Transport) DialRPC(ctx context.Context, peer *protocol.Node, hs rpc.RPCHandshakeFunc) (rpc.RPC, error) {
	args := t.Called(ctx, peer, hs)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(rpc.RPC), e
}

func (t *Transport) DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error) {
	args := t.Called(ctx, peer)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(net.Conn), e
}

func (t *Transport) RPC() <-chan *transport.StreamDelegate {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *transport.StreamDelegate)
}

func (t *Transport) Direct() <-chan *transport.StreamDelegate {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *transport.StreamDelegate)
}

func (t *Transport) SupportDatagram() bool {
	args := t.Called()
	v := args.Bool(0)
	return v
}

func (t *Transport) ReceiveDatagram() <-chan *transport.DatagramDelegate {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *transport.DatagramDelegate)
}

func (t *Transport) SendDatagram(n *protocol.Node, b []byte) error {
	args := t.Called(n, b)
	e := args.Error(0)
	return e
}

func (t *Transport) TransportEstablished() <-chan *protocol.Node {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *protocol.Node)
}

func (t *Transport) TransportDestroyed() <-chan *protocol.Node {
	args := t.Called()
	v := args.Get(0)
	return v.(chan *protocol.Node)
}

func (t *Transport) Accept(ctx context.Context) error {
	args := t.Called(ctx)
	e := args.Error(0)
	return e
}

func (t *Transport) Stop() {
	t.Called()
}
