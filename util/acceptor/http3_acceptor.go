package acceptor

import (
	"context"
	"net"

	"go.miragespace.co/specter/spec/transport/q"

	"github.com/quic-go/quic-go"
	"go.uber.org/atomic"
)

type HTTP3Acceptor struct {
	parent  q.Listener
	conn    chan *quic.Conn
	closeCh chan struct{}
	closed  atomic.Bool
}

var _ q.Listener = (*HTTP3Acceptor)(nil)

func NewH3Acceptor(parent q.Listener) *HTTP3Acceptor {
	return &HTTP3Acceptor{
		parent:  parent,
		conn:    make(chan *quic.Conn, 128),
		closeCh: make(chan struct{}),
	}
}

func (h *HTTP3Acceptor) Handle(c *quic.Conn) {
	h.conn <- c
}

func (h *HTTP3Acceptor) Accept(ctx context.Context) (*quic.Conn, error) {
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
		return emptyAddr
	}
	return h.parent.Addr()
}
