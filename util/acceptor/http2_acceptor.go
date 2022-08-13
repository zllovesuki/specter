package acceptor

import (
	"net"

	"go.uber.org/atomic"
)

type HTTP2Acceptor struct {
	parent  net.Listener
	conn    chan net.Conn
	closeCh chan struct{}
	closed  atomic.Bool
}

var _ net.Listener = &HTTP2Acceptor{}

func NewH2Acceptor(parent net.Listener) *HTTP2Acceptor {
	return &HTTP2Acceptor{
		parent:  parent,
		conn:    make(chan net.Conn, 128),
		closeCh: make(chan struct{}),
	}
}

func (h *HTTP2Acceptor) Handle(c net.Conn) {
	h.conn <- c
}

func (h *HTTP2Acceptor) Accept() (net.Conn, error) {
	select {
	case <-h.closeCh:
		return nil, net.ErrClosed
	case c := <-h.conn:
		return c, nil
	}
}

func (h *HTTP2Acceptor) Close() error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(h.closeCh)
	return nil
}

func (h *HTTP2Acceptor) Addr() net.Addr {
	if h.parent == nil {
		return emptyAddr
	}
	return h.parent.Addr()
}
