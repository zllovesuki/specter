package acceptor

import (
	"net"

	"go.uber.org/atomic"
)

type HTTP2Acceptor struct {
	Conn    chan net.Conn
	parent  net.Listener
	closed  atomic.Bool
	closeCh chan struct{}
}

var _ net.Listener = &HTTP2Acceptor{}

func NewH2Acceptor(parent net.Listener) *HTTP2Acceptor {
	return &HTTP2Acceptor{
		parent:  parent,
		Conn:    make(chan net.Conn, 128),
		closeCh: make(chan struct{}),
	}
}

func (h *HTTP2Acceptor) Accept() (net.Conn, error) {
	select {
	case <-h.closeCh:
		return nil, net.ErrClosed
	case c := <-h.Conn:
		return c, nil
	}
}

func (h *HTTP2Acceptor) Close() error {
	if !h.closed.CAS(false, true) {
		return nil
	}
	close(h.closeCh)
	return nil
}

func (h *HTTP2Acceptor) Addr() net.Addr {
	if h.parent == nil {
		return nil
	}
	return h.parent.Addr()
}
