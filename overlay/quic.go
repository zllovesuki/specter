package overlay

import (
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

var (
	quicConfig = &quic.Config{
		KeepAlivePeriod:      time.Second * 5,
		HandshakeIdleTimeout: time.Second * 3,
		MaxIdleTimeout:       time.Second * 30,
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

func (q *quicConn) Close() error {
	// https://github.com/lucas-clemente/quic-go/issues/3558#issuecomment-1253315560
	q.Stream.CancelRead(409)
	return q.Stream.Close()
}

func WrapQuicConnection(s quic.Stream, q quic.Connection) net.Conn {
	return &quicConn{
		Stream: s,
		q:      q,
	}
}
