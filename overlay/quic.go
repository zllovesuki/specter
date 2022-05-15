package overlay

import (
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"
)

var (
	quicConfig = &quic.Config{
		KeepAlive:            true,
		HandshakeIdleTimeout: time.Second * 3,
		MaxIdleTimeout:       time.Second * 5,
		EnableDatagrams:      true,
	}
)

type quicConn struct {
	quic.Stream
	q quic.Connection
}

var _ net.Conn = (*quicConn)(nil)

func (q *quicConn) LocalAddr() net.Addr {
	return q.q.LocalAddr()
}

func (q *quicConn) RemoteAddr() net.Addr {
	return q.q.RemoteAddr()
}
