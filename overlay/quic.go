package overlay

import (
	"net"
	"sync"
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
	cbOnce sync.Once
	quic.Stream
	q       quic.Connection
	closeCb func()
}

var _ net.Conn = (*quicConn)(nil)

func (q *quicConn) LocalAddr() net.Addr {
	return q.q.LocalAddr()
}

func (q *quicConn) RemoteAddr() net.Addr {
	return q.q.RemoteAddr()
}

func (q *quicConn) Close() error {
	if q.closeCb != nil {
		defer q.cbOnce.Do(q.closeCb)
	}
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
