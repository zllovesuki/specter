//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"context"
	"net"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"

	"github.com/stretchr/testify/mock"
)

type TunnelServer struct {
	mock.Mock
}

var _ tun.Server = (*TunnelServer)(nil)

func (m *TunnelServer) Identity() *protocol.Node {
	args := m.Called()
	n := args.Get(0)
	if n == nil {
		return nil
	}
	return n.(*protocol.Node)
}

func (m *TunnelServer) Dial(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	args := m.Called(ctx, link)
	c := args.Get(0)
	e := args.Error(1)
	if c == nil {
		return nil, e
	}
	return c.(net.Conn), e
}

func (m *TunnelServer) DialInternal(ctx context.Context, node *protocol.Node) (net.Conn, error) {
	args := m.Called(ctx, node)
	c := args.Get(0)
	e := args.Error(1)
	if c == nil {
		return nil, e
	}
	return c.(net.Conn), e
}
