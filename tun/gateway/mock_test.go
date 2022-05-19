package gateway

import (
	"context"
	"net"

	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/tun"

	"github.com/stretchr/testify/mock"
)

type mockServer struct {
	mock.Mock
}

var _ tun.Server = (*mockServer)(nil)

func (m *mockServer) Dial(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	args := m.Called(ctx, link)
	c := args.Get(0)
	if c == nil {
		return nil, args.Error(1)
	}
	return c.(net.Conn), args.Error(1)
}

func (m *mockServer) Accept(ctx context.Context) {
	m.Called(ctx)
}

func (m *mockServer) Stop() {
	m.Called()
}
