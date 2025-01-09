package overlay

import (
	"net"
	"time"

	"go.miragespace.co/specter/timing"

	"github.com/quic-go/quic-go"
)

var (
	quicConfig = &quic.Config{
		KeepAlivePeriod:      time.Second * 5,
		HandshakeIdleTimeout: timing.TLSHandshakeTimeout,
		MaxIdleTimeout:       time.Second * 30,
		MaxIncomingStreams:   500,
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
	go func() {
		time.Sleep(time.Second)
		q.Stream.CancelWrite(409)
	}()
	q.Stream.CancelRead(409)
	return q.Stream.Close()
}

func WrapQuicConnection(s quic.Stream, q quic.Connection) net.Conn {
	return &quicConn{
		Stream: s,
		q:      q,
	}
}
