package tun

import (
	"net"
)

type HTTP2Acceptor struct {
	Parent net.Listener
	Conn   chan net.Conn
}

var _ net.Listener = &HTTP2Acceptor{}

func (h *HTTP2Acceptor) Accept() (net.Conn, error) {
	c := <-h.Conn
	if c == nil {
		return nil, net.ErrClosed
	}
	return c, nil
}

func (h *HTTP2Acceptor) Close() error {
	if h.Parent == nil {
		return nil
	}
	return h.Parent.Close()
}

func (h *HTTP2Acceptor) Addr() net.Addr {
	if h.Parent == nil {
		return nil
	}
	return h.Parent.Addr()
}
