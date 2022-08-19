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

type TunServer struct {
	mock.Mock
}

var _ tun.Server = (*TunServer)(nil)

func (m *TunServer) Dial(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	args := m.Called(ctx, link)
	c := args.Get(0)
	e := args.Error(1)
	if c == nil {
		return nil, e
	}
	return c.(net.Conn), e
}
