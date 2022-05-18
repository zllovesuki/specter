package tun

import (
	"io"
	"net"
)

type HTTPAcceptor struct {
	Parent net.Listener
	Conn   chan net.Conn
}

var _ net.Listener = &HTTPAcceptor{}

func (h *HTTPAcceptor) Accept() (net.Conn, error) {
	c := <-h.Conn
	if c == nil {
		return nil, io.EOF
	}
	return c, nil
}

func (h *HTTPAcceptor) Close() error {
	if h.Parent == nil {
		return nil
	}
	return h.Parent.Close()
}

func (h *HTTPAcceptor) Addr() net.Addr {
	if h.Parent == nil {
		return nil
	}
	return h.Parent.Addr()
}
