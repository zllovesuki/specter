package tun

import (
	"context"
	"net"

	"github.com/lucas-clemente/quic-go"
)

type HTTP3Acceptor struct {
	Parent quic.EarlyListener
	Conn   chan quic.EarlyConnection
}

var _ quic.EarlyListener = &HTTP3Acceptor{}

func (h *HTTP3Acceptor) Accept(ctx context.Context) (quic.EarlyConnection, error) {
	select {
	case <-ctx.Done():
		return nil, net.ErrClosed
	case c := <-h.Conn:
		if c == nil {
			return nil, net.ErrClosed
		}
		return c, nil
	}
}

func (h *HTTP3Acceptor) Close() error {
	if h.Parent == nil {
		return nil
	}
	return h.Parent.Close()
}

func (h *HTTP3Acceptor) Addr() net.Addr {
	if h.Parent == nil {
		return nil
	}
	return h.Parent.Addr()
}
