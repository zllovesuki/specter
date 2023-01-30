package overlay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util/atomic"

	"github.com/avast/retry-go/v4"
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
	var (
		qKey = makeCachedKey(peer)
		q    quic.EarlyConnection
	)

	if err := retry.Do(func() error {
		rUnlock := t.cachedMutex.RLock(qKey)
		if cached, ok := t.cachedConnections.Load(qKey); ok {
			rUnlock()
			q = cached.quic
			return nil
		}
		rUnlock()

		if peer.GetAddress() == "" {
			return transport.ErrNoDirect
		}

		t.Logger.Debug("Creating new QUIC connection", zap.String("peer", peer.String()))

		dialCtx, dialCancel := context.WithTimeout(ctx, transport.ConnectTimeout)
		defer dialCancel()

		newQ, err := quic.DialAddrEarlyContext(dialCtx, peer.GetAddress(), t.ClientTLS, quicConfig)
		if err != nil {
			return err
		}

		reused, err := t.handleOutgoing(ctx, newQ)
		if err != nil {
			return err
		}

		q = reused
		return nil
	},
		retry.Attempts(2),
		retry.Context(ctx),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			t.Logger.Info("Potential connection reuse conflict, retrying to get previously cached connection", zap.String("peer", peer.String()), zap.Error(err))
		}),
		retry.RetryIf(func(err error) bool {
			return strings.Contains(err.Error(), "invalid state")
		}),
	); err != nil {
		t.Logger.Error("Failed to establish connection", zap.String("peer", peer.String()), zap.Error(err))
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

	err := rpc.Send(s, negotiation)
	if err != nil {
		return nil, false, fmt.Errorf("error sending identity: %w", err)
	}
	negotiation.Reset()

	s.SetReadDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpc.BoundedReceive(s, negotiation, 1024)
	if err != nil {
		return nil, false, fmt.Errorf("error receiving identity: %w", err)
	}
	s.SetReadDeadline(time.Time{})

	qKey := makeCachedKey(negotiation.GetIdentity())
	fresh := &nodeConnection{
		peer:      negotiation.GetIdentity(),
		quic:      q,
		direction: dir,
	}

	if t.Endpoint.GetId() == negotiation.GetIdentity().GetId() {
		t.Logger.Debug("connecting to self, skipping connection reuse", zap.String("direction", dir.String()))
		return fresh, false, nil
	}

	negotiation.Reset()

	rUnlock := t.cachedMutex.RLock(qKey)
	cache, cached := t.cachedConnections.Load(qKey)
	if cached {
		negotiation.CacheState = protocol.Connection_CACHED
		if cache.direction == directionIncoming {
			negotiation.CacheDirection = protocol.Connection_INCOMING
		} else {
			negotiation.CacheDirection = protocol.Connection_OUTGOING
		}
	} else {
		negotiation.CacheState = protocol.Connection_FRESH
		if dir == directionIncoming {
			negotiation.CacheDirection = protocol.Connection_INCOMING
		} else {
			negotiation.CacheDirection = protocol.Connection_OUTGOING
		}
	}
	rUnlock()

	err = rpc.Send(s, negotiation)
	if err != nil {
		return nil, false, fmt.Errorf("error sending cache status: %w", err)
	}
	negotiation.Reset()

	s.SetReadDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpc.BoundedReceive(s, negotiation, 8)
	if err != nil {
		return nil, false, fmt.Errorf("error receiving cache status: %w", err)
	}
	s.SetReadDeadline(time.Time{})

	unlock := t.cachedMutex.Lock(qKey)
	defer unlock()

	// I really should make a state machine for this
	switch negotiation.CacheState {
	case protocol.Connection_CACHED:
		switch negotiation.CacheDirection {
		case protocol.Connection_INCOMING:
			if cached {
				if cache.direction == directionIncoming {
					// other: cached incoming
					//    us: cached incoming
					return nil, false, fmt.Errorf("invalid state: both peers have cached incoming connections")
				} else {
					// other: cached incoming
					//    us: cached outgoing
					fresh.quic.CloseWithError(508, "Previously cached connection was reused")
					return cache, true, nil
				}
			} else {
				if dir == directionIncoming {
					// other: cached incoming
					//    us:    new incoming
					return nil, false, fmt.Errorf("invalid state: both peers have incoming connections")
				} else {
					// other: cached incoming
					//    us:    new outgoing
					return nil, false, fmt.Errorf("invalid state: other peer has cached connection while we are establishing a new outgoing connection")
				}
			}
		case protocol.Connection_OUTGOING:
			if cached {
				if cache.direction == directionIncoming {
					// other: cached outgoing
					//    us: cached incoming
					// we will let the receiver side close the connection
					return cache, true, nil
				} else {
					// other: cached outgoing
					//    us: cached outgoing
					return nil, false, fmt.Errorf("invalid state: both peers have cached outgoing connections")
				}
			} else {
				if dir == directionIncoming {
					// other: cached outgoing
					//    us:    new incoming
					return nil, false, fmt.Errorf("invalid state: other peer has cached connection while we are handling a new incoming connection")
				} else {
					// other: cached outgoing
					//    us:    new outgoing
					return nil, false, fmt.Errorf("invalid state: both peers have outgoing connections")
				}
			}
		default:
			return nil, false, fmt.Errorf("invalid state: unknown transport cache direction")
		}
	case protocol.Connection_FRESH:
		switch negotiation.CacheDirection {
		case protocol.Connection_INCOMING:
			if cached {
				if cache.direction == directionIncoming {
					// other:    new incoming
					//    us: cached incoming
					return nil, false, fmt.Errorf("invalid state: both peers have cached incoming connections")
				} else {
					// other:    new incoming
					//    us: cached outgoing
					return nil, false, fmt.Errorf("invalid state: we have cached connection while peer is handling a new incoming connection")
				}
			} else {
				if dir == directionIncoming {
					// other:    new incoming
					//    us:    new incoming
					return nil, false, fmt.Errorf("invalid state: both peers have incoming connections")
				} else {
					// other:    new incoming
					//    us:    new outgoing
					t.cachedConnections.Store(qKey, fresh)
					return fresh, false, nil
				}
			}
		case protocol.Connection_OUTGOING:
			if cached {
				if cache.direction == directionIncoming {
					// other:    new outgoing
					//    us: cached incoming
					// it is likely that concurrent reuse was issued by the other peer, therefore they should
					// try again with the cached connection
					return nil, false, fmt.Errorf("invalid state: we have cached connection while peer is establishing a new outgoing connection")
				} else {
					// other:    new outgoing
					//    us: cached outgoing
					return nil, false, fmt.Errorf("invalid state: both peers have outgoing connections")
				}
			} else {
				if dir == directionIncoming {
					// other:    new outgoing
					//    us:    new incoming
					t.cachedConnections.Store(qKey, fresh)
					return fresh, false, nil
				} else {
					// other:    new outgoing
					//    us:    new outgoing
					return nil, false, fmt.Errorf("invalid state: both peers have outgoing connections")
				}
			}
		default:
			return nil, false, fmt.Errorf("invalid state: unknown transport cache direction")
		}
	default:
		return nil, false, fmt.Errorf("invalid state: unknown transport cache state")
	}
}

func (t *QUIC) handleIncoming(ctx context.Context, q quic.EarlyConnection) (quic.EarlyConnection, error) {
	openCtx, openCancel := context.WithTimeout(ctx, quicConfig.HandshakeIdleTimeout)
	defer openCancel()

	stream, err := q.OpenStreamSync(openCtx)
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

	stream, err := q.AcceptStream(openCtx)
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
		go func(q quic.EarlyConnection) {
			if _, err := t.handleIncoming(ctx, q); err != nil {
				t.Logger.Error("Incoming connection reuse error", zap.String("endpoint", q.RemoteAddr().String()), zap.Error(err))
				// TODO: figure out a better way to ensure that the peer received cache status before closing
				time.Sleep(time.Second)
				q.CloseWithError(406, err.Error())
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
		Conn:     WrapQuicConnection(stream, q),
		Identity: peer,
		Kind:     rr.GetType(),
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
