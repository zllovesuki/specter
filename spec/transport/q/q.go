package q

import (
	"context"
	"net"

	"github.com/quic-go/quic-go"
)

// Listener interface for accepting QUIC connections
type Listener interface {
	Accept(context.Context) (*quic.Conn, error)
	Addr() net.Addr
	Close() error
}
