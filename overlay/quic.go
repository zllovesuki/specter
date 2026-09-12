package overlay

import (
	"net"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"
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
	*quic.Stream
	q *quic.Conn
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

func WrapQuicConnection(s *quic.Stream, q *quic.Conn) net.Conn {
	return &quicConn{
		Stream: s,
		q:      q,
	}
}

// attachment identifies the QUIC connection itself, not its reusable cache key.
type attachment struct {
	q *quic.Conn
}

func (a attachment) Done() <-chan struct{} { return a.q.Context().Done() }

func (a attachment) Err() error { return a.q.Context().Err() }

func (a attachment) Close(reason string) error { return a.q.CloseWithError(410, reason) }

func (a attachment) OpenStream(kind protocol.Stream_Type) (net.Conn, error) {
	return openStream(a.q, &protocol.Stream{Type: kind})
}

func (c *quicConn) PhysicalConn() transport.PhysicalConn { return attachment{c.q} }
