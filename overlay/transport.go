package overlay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util/atomic"

	"github.com/quic-go/quic-go"
	"github.com/zhangyunhao116/skipmap"
	uberAtomic "go.uber.org/atomic"
	"go.uber.org/zap"
)

var _ transport.Transport = (*QUIC)(nil)

func NewQUIC(conf TransportConfig) *QUIC {
	return &QUIC{
		TransportConfig: conf,

		cachedConnections: skipmap.NewString[*nodeConnection](),
		cachedMutex:       atomic.NewKeyedRWMutex(),

		streamChan: make(chan *transport.StreamDelegate, 32),
		dgramChan:  make(chan *transport.DatagramDelegate, 32),

		started: uberAtomic.NewBool(false),
		closed:  uberAtomic.NewBool(false),
	}
}

func makeCachedKey(peer *protocol.Node) string {
	qMapKey := peer.GetAddress() + "/"
	if peer.GetUnknown() {
		qMapKey = qMapKey + "-1"
	} else {
		qMapKey = qMapKey + strconv.FormatUint(peer.GetId(), 10)
	}
	return qMapKey
}

func (t *QUIC) getCachedConnection(ctx context.Context, peer *protocol.Node) (quic.EarlyConnection, error) {
	qKey := makeCachedKey(peer)

	rUnlock := t.cachedMutex.RLock(qKey)
	if sQ, ok := t.cachedConnections.Load(qKey); ok {
		rUnlock()
		// t.Logger.Debug("Reusing quic connection from reuseMap", zap.String("key", qKey))
		return sQ.quic, nil
	}
	rUnlock()

	if peer.GetAddress() == "" {
		return nil, transport.ErrNoDirect
	}

	t.Logger.Debug("Creating new QUIC connection", zap.String("peer", peer.String()))

	dialCtx, dialCancel := context.WithTimeout(ctx, transport.ConnectTimeout)
	defer dialCancel()

	q, err := quic.DialAddrEarlyContext(dialCtx, peer.GetAddress(), t.ClientTLS, quicConfig)
	if err != nil {
		return nil, err
	}

	q, err = t.handleOutgoing(ctx, q)
	if err != nil {
		return nil, err
	}

	t.background(ctx)

	return q, nil
}

func (t *QUIC) Identity() *protocol.Node {
	return t.Endpoint
}

func (t *QUIC) DialStream(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
	if t.closed.Load() {
		return nil, transport.ErrClosed
	}

	q, err := t.getCachedConnection(ctx, peer)
	if err != nil {
		return nil, fmt.Errorf("creating quic connection: %w", err)
	}

	stream, err := q.OpenStream()
	if err != nil {
		return nil, err
	}

	rr := &protocol.Stream{
		Type: kind,
	}
	stream.SetDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpc.Send(stream, rr)
	if err != nil {
		return nil, err
	}
	stream.SetDeadline(time.Time{})

	// t.Logger.Debug("Created new Stream",
	// 	zap.String("kind", kind.String()),
	// 	zap.String("remote", q.RemoteAddr().String()),
	// 	zap.String("local", q.LocalAddr().String()))

	return WrapQuicConnection(stream, q), nil
}

func (t *QUIC) SupportDatagram() bool {
	return quicConfig.EnableDatagrams
}

func (t *QUIC) ReceiveDatagram() <-chan *transport.DatagramDelegate {
	return t.dgramChan
}

func (t *QUIC) SendDatagram(peer *protocol.Node, buf []byte) error {
	qKey := makeCachedKey(peer)
	if r, ok := t.cachedConnections.Load(qKey); ok {
		data := &protocol.Datagram{
			Type: protocol.Datagram_DATA,
			Data: buf,
		}
		b, err := data.MarshalVT()
		if err != nil {
			return err
		}
		return r.quic.SendMessage(b)
	}
	return transport.ErrNoDirect
}

func (t *QUIC) reuseConnection(ctx context.Context, q quic.EarlyConnection, s quic.Stream, dir direction) (*nodeConnection, bool, error) {
	negotiation := &protocol.Connection{
		Identity: t.Endpoint,
	}

	s.SetDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err := rpc.Send(s, negotiation)
	if err != nil {
		return nil, false, err
	}
	negotiation.Reset()
	err = rpc.BoundedReceive(s, negotiation, 1024)
	if err != nil {
		return nil, false, err
	}
	s.SetDeadline(time.Time{})

	qKey := makeCachedKey(negotiation.GetIdentity())
	fresh := &nodeConnection{
		peer: negotiation.GetIdentity(),
		quic: q,
	}

	if t.Endpoint.GetId() == negotiation.GetIdentity().GetId() {
		t.Logger.Debug("connecting to self, skipping connection reuse", zap.String("direction", dir.String()))
		return fresh, false, nil
	}

	negotiation.Reset()

	unlock := t.cachedMutex.Lock(qKey)
	defer unlock()

	cached, loaded := t.cachedConnections.Load(qKey)
	if loaded {
		negotiation.CacheState = protocol.Connection_CACHED
	} else {
		negotiation.CacheState = protocol.Connection_FRESH
	}

	s.SetDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpc.Send(s, negotiation)
	if err != nil {
		return nil, false, err
	}
	negotiation.Reset()
	err = rpc.BoundedReceive(s, negotiation, 8)
	if err != nil {
		return nil, false, err
	}
	s.SetDeadline(time.Time{})

	l := t.Logger.With(zap.String("direction", dir.String()), zap.String("key", qKey))
	switch negotiation.CacheState {
	case protocol.Connection_CACHED:
		if loaded {
			l.Debug("Reusing quic connection: both sides have cached connections")
			fresh.quic.CloseWithError(0, "A previous connection was reused")
			return cached, true, nil
		} else {
			l.Debug("Caching quic connection: other side has cached connection")
			t.cachedConnections.Store(qKey, fresh)
			return fresh, false, nil
		}
	case protocol.Connection_FRESH:
		if loaded {
			l.Debug("Reusing cached quic connection: this side has cached connection")
			fresh.quic.CloseWithError(0, "A new connection is reused")
			return cached, true, nil
		} else {
			l.Debug("Caching quic connection: this side has no cached connection")
			t.cachedConnections.Store(qKey, fresh)
			return fresh, false, nil
		}
	default:
		return nil, false, fmt.Errorf("unknown transport cache state")
	}
}

func (t *QUIC) handleIncoming(ctx context.Context, q quic.EarlyConnection) (quic.EarlyConnection, error) {
	openCtx, openCancel := context.WithTimeout(ctx, quicConfig.HandshakeIdleTimeout)
	defer openCancel()

	stream, err := q.AcceptStream(openCtx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	c, reused, err := t.reuseConnection(ctx, q, stream, directionIncoming)
	if err != nil {
		return nil, err
	}

	if !reused {
		t.handlePeer(ctx, c.quic, c.peer, directionIncoming)
	}

	return c.quic, nil
}

func (t *QUIC) handleOutgoing(ctx context.Context, q quic.EarlyConnection) (quic.EarlyConnection, error) {
	openCtx, openCancel := context.WithTimeout(ctx, quicConfig.HandshakeIdleTimeout)
	defer openCancel()

	stream, err := q.OpenStreamSync(openCtx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	c, reused, err := t.reuseConnection(ctx, q, stream, directionOutgoing)
	if err != nil {
		return nil, err
	}

	if !reused && c.peer.GetId() != t.Endpoint.GetId() {
		t.handlePeer(ctx, c.quic, c.peer, directionOutgoing)
	}

	return c.quic, nil
}

func (t *QUIC) handlePeer(ctx context.Context, q quic.EarlyConnection, peer *protocol.Node, dir direction) {
	l := t.Logger.With(
		zap.String("remote", q.RemoteAddr().String()),
		zap.String("peer", peer.String()),
		zap.String("direction", dir.String()),
		zap.String("key", makeCachedKey(peer)),
	)
	l.Debug("Starting goroutines to handle streams and datagrams")
	go t.handleConnection(ctx, q, peer)
	go t.handleDatagram(ctx, q, peer)
	go func(q quic.Connection) {
		<-q.Context().Done()
		l.Info("Connection with peer closed", zap.Error(q.Context().Err()))
		t.reapPeer(q, peer)
	}(q)
}

func (t *QUIC) background(ctx context.Context) {
	if !t.started.CompareAndSwap(false, true) {
		return
	}
	go t.reaper(ctx)
}

func (t *QUIC) AcceptWithListener(ctx context.Context, listener quic.EarlyListener) error {
	t.Logger.Info("Accepting connections", zap.String("listen", listener.Addr().String()))
	t.background(ctx)
	for {
		q, err := listener.Accept(ctx)
		if err != nil {
			return err
		}
		go func(prev quic.EarlyConnection) {
			if _, err := t.handleIncoming(ctx, prev); err != nil {
				t.Logger.Error("incoming connection reuse error", zap.Error(err))
			}
		}(q)
	}
}

func (t *QUIC) Accept(ctx context.Context) error {
	if t.ServerTLS == nil {
		return fmt.Errorf("missing ServerTLS")
	}
	l, err := quic.ListenAddrEarly(t.Endpoint.GetAddress(), t.ServerTLS, quicConfig)
	if err != nil {
		return err
	}
	return t.AcceptWithListener(ctx, l)
}

func (t *QUIC) AcceptStream() <-chan *transport.StreamDelegate {
	return t.streamChan
}

func (t *QUIC) handleDatagram(ctx context.Context, q quic.Connection, peer *protocol.Node) {
	logger := t.Logger.With(zap.String("endpoint", q.RemoteAddr().String()))
	for {
		b, err := q.ReceiveMessage()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				logger.Error("error receiving datagram", zap.Error(err))
			}
			return
		}
		data := &protocol.Datagram{}
		if err := data.UnmarshalVT(b); err != nil {
			logger.Error("error decoding datagram to proto", zap.Error(err))
			continue
		}
		switch data.GetType() {
		case protocol.Datagram_ALIVE:
		case protocol.Datagram_DATA:
			select {
			case t.dgramChan <- &transport.DatagramDelegate{Buffer: data.GetData(), Identity: peer}:
			default:
				logger.Warn("datagram data buffer full, dropping datagram")
			}
		default:
			logger.Warn("unknown datagram type: %s", zap.String("type", data.GetType().String()))
		}
	}
}

func (t *QUIC) handleConnection(ctx context.Context, q quic.Connection, peer *protocol.Node) {
	for {
		stream, err := q.AcceptStream(ctx)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				t.Logger.Error("Error accepting new stream from peer", zap.String("peer", peer.String()), zap.String("remote", q.RemoteAddr().String()), zap.Error(err))
			}
			return
		}
		go t.streamHandler(q, stream, peer)
	}
}

func (t *QUIC) streamHandler(q quic.Connection, stream quic.Stream, peer *protocol.Node) {
	l := t.Logger.With(zap.String("peer", peer.String()))

	var err error
	defer func() {
		if err != nil {
			l.Error("error handshaking on new stream", zap.Error(err))
			stream.Close()
		}
	}()

	rr := &protocol.Stream{}
	stream.SetDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpc.BoundedReceive(stream, rr, 8)
	if err != nil {
		l.Error("Failed to receive stream handshake", zap.Error(err))
		return
	}
	stream.SetDeadline(time.Time{})

	if rr.GetType() == protocol.Stream_UNKNOWN_TYPE {
		l.Warn("Received stream with unknown type")
		stream.Close()
		return
	}

	select {
	case t.streamChan <- &transport.StreamDelegate{
		Connection: WrapQuicConnection(stream, q),
		Identity:   peer,
		Kind:       rr.GetType(),
	}:
	default:
		l.Warn("Stream channel full, dropping incoming stream",
			zap.String("kind", rr.GetType().String()),
		)
		stream.Close()
	}
}

func (t *QUIC) Stop() {
	if !t.closed.CompareAndSwap(false, true) {
		return
	}
	t.started.Store(false)
	t.cachedConnections.Range(func(key string, value *nodeConnection) bool {
		value.quic.CloseWithError(0, "Transport closed")
		return true
	})
}
