package acceptor

import (
	"context"
	"net"

	"github.com/lucas-clemente/quic-go"
	"go.uber.org/atomic"
)

type HTTP3Acceptor struct {
	parent  quic.EarlyListener
	conn    chan quic.EarlyConnection
	closeCh chan struct{}
	closed  atomic.Bool
}

var _ quic.EarlyListener = &HTTP3Acceptor{}

func NewH3Acceptor(parent quic.EarlyListener) *HTTP3Acceptor {
	return &HTTP3Acceptor{
		parent:  parent,
		conn:    make(chan quic.EarlyConnection, 128),
		closeCh: make(chan struct{}),
	}
}

func (h *HTTP3Acceptor) Handle(c quic.EarlyConnection) {
	h.conn <- c
}

func (h *HTTP3Acceptor) Accept(ctx context.Context) (quic.EarlyConnection, error) {
	select {
	case <-ctx.Done():
		return nil, net.ErrClosed
	case <-h.closeCh:
		return nil, net.ErrClosed
	case c := <-h.conn:
		return c, nil
	}
}

func (h *HTTP3Acceptor) Close() error {
	if !h.closed.CompareAndSwap(false, true) {
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
