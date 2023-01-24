//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"context"
	"net"

	"kon.nect.sh/specter/spec/protocol"
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

func (t *Transport) Dial(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
	args := t.Called(ctx, peer, kind)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(net.Conn), e
}

func (t *Transport) AcceptStream() <-chan *transport.StreamDelegate {
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
