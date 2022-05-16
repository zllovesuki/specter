package gateway

import (
	"io"
	"net"
)

type httpAccepter struct {
	parent net.Listener
	ch     chan net.Conn
}

var _ net.Listener = &httpAccepter{}

func (h *httpAccepter) Accept() (net.Conn, error) {
	c := <-h.ch
	if c == nil {
		return nil, io.EOF
	}
	return c, nil
}

func (h *httpAccepter) Close() error {
	return nil
}

func (h *httpAccepter) Addr() net.Addr {
	return h.parent.Addr()
}
