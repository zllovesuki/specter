package overlay

import (
	"net"

	"github.com/lucas-clemente/quic-go"
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
