package acceptor

import (
	"context"
	"net"

	"github.com/lucas-clemente/quic-go"
	"go.uber.org/atomic"
)

type HTTP3Acceptor struct {
	Conn    chan quic.EarlyConnection
	parent  quic.EarlyListener
	closed  atomic.Bool
	closeCh chan struct{}
}

var _ quic.EarlyListener = &HTTP3Acceptor{}

func NewH3Acceptor(parent quic.EarlyListener) *HTTP3Acceptor {
	return &HTTP3Acceptor{
		parent:  parent,
		Conn:    make(chan quic.EarlyConnection, 128),
		closeCh: make(chan struct{}),
	}
}

func (h *HTTP3Acceptor) Accept(ctx context.Context) (quic.EarlyConnection, error) {
	select {
	case <-ctx.Done():
		return nil, net.ErrClosed
	case <-h.closeCh:
		return nil, net.ErrClosed
	case c := <-h.Conn:
		return c, nil
	}
}

func (h *HTTP3Acceptor) Close() error {
	if !h.closed.CAS(false, true) {
		return nil
	}
	close(h.closeCh)
	return nil
}

func (h *HTTP3Acceptor) Addr() net.Addr {
	if h.parent == nil {
		return nil
	}
	return h.parent.Addr()
}
