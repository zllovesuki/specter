package overlay

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/protocol"
	rpcSpec "kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util/atomic"

	"github.com/lucas-clemente/quic-go"
	"github.com/zhangyunhao116/skipmap"
	uberAtomic "go.uber.org/atomic"
	"go.uber.org/zap"
)

var _ transport.Transport = (*QUIC)(nil)

func NewQUIC(conf TransportConfig) *QUIC {
	return &QUIC{
		TransportConfig: conf,

		qMap:   skipmap.NewString[*nodeConnection](),
		qMu:    atomic.NewKeyedRWMutex(),
		rpcMap: skipmap.NewString[rpcSpec.RPC](),
		rpcMu:  atomic.NewKeyedRWMutex(),

		rpcChan:    make(chan *transport.StreamDelegate),
		directChan: make(chan *transport.StreamDelegate),
		dgramChan:  make(chan *transport.DatagramDelegate, 32),

		started: uberAtomic.NewBool(false),
		closed:  uberAtomic.NewBool(false),
	}
}

func makeQKey(peer *protocol.Node) string {
	qMapKey := peer.GetAddress() + "/"
	if peer.GetUnknown() {
		qMapKey = qMapKey + "-1"
	} else {
		qMapKey = qMapKey + strconv.FormatUint(peer.GetId(), 10)
	}
	return qMapKey
}

func makeSKey(peer *protocol.Node) string {
	if peer.GetAddress() == "" {
		return "/" + strconv.FormatUint(peer.GetId(), 10)
	} else {
		return "/" + peer.GetAddress()
	}
}

func (t *QUIC) getQ(ctx context.Context, peer *protocol.Node) (quic.EarlyConnection, error) {
	qKey := makeQKey(peer)

	rUnlock := t.qMu.RLock(qKey)
	if sQ, ok := t.qMap.Load(qKey); ok {
		rUnlock()
		t.Logger.Debug("Reusing quic connection from reuseMap", zap.String("key", qKey))
		return sQ.quic, nil
	}
	rUnlock()

	if peer.GetAddress() == "" {
		return nil, transport.ErrNoDirect
	}

	t.Logger.Debug("Creating new QUIC connection", zap.Any("peer", peer))

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

func (t *QUIC) getS(ctx context.Context, peer *protocol.Node, sType protocol.Stream_Type) (q quic.EarlyConnection, stream quic.Stream, err error) {
	// defer func() {
	// 	if err != nil {
	// 		t.Logger.Error("Dialing new stream", zap.Error(err), zap.String("type", sType.String()), zap.String("addr", peer.GetAddress()))
	// 	}
	// }()

	q, err = t.getQ(ctx, peer)
	if err != nil {
		return nil, nil, fmt.Errorf("creating quic connection: %w", err)
	}

	openCtx, openCancel := context.WithTimeout(ctx, transport.ConnectTimeout)
	defer openCancel()

	stream, err = q.OpenStreamSync(openCtx)
	if err != nil {
		return nil, nil, err
	}

	rr := &protocol.Stream{
		Type: sType,
	}
	stream.SetDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpcSpec.Send(stream, rr)
	if err != nil {
		return
	}
	stream.SetDeadline(time.Time{})

	return q, stream, nil
}

func (t *QUIC) Identity() *protocol.Node {
	return t.Endpoint
}

func (t *QUIC) DialRPC(ctx context.Context, peer *protocol.Node, hs rpcSpec.RPCHandshakeFunc) (rpcSpec.RPC, error) {
	if t.closed.Load() {
		return nil, transport.ErrClosed
	}

	rpcMapKey := makeSKey(peer)

	rUnlock := t.rpcMu.RLock(rpcMapKey)
	if r, ok := t.rpcMap.Load(rpcMapKey); ok {
		rUnlock()
		return r, nil
	}
	rUnlock()

	// t.Logger.Debug("=== DialRPC B ===", zap.String("peer", makeQKey(peer)))

	unlock := t.rpcMu.Lock(rpcMapKey)
	defer func() {
		unlock()
	}()

	if r, ok := t.rpcMap.Load(rpcMapKey); ok {
		return r, nil
	}

	// t.Logger.Debug("=== DialRPC C ===", zap.String("peer", makeQKey(peer)))

	q, stream, err := t.getS(ctx, peer, protocol.Stream_RPC)
	if err != nil {
		return nil, fmt.Errorf("dialing stream: %w", err)
	}

	l := t.Logger.With(
		zap.Any("peer", peer),
		zap.String("remote", q.RemoteAddr().String()),
		zap.String("local", q.LocalAddr().String()))

	l.Debug("Created new RPC Stream")

	r := rpc.NewRPC(
		l.With(zap.String("pov", "transport_dial")),
		&quicConn{
			Stream: stream,
			q:      q,
			closeCb: func() {
				t.rpcMap.Delete(rpcMapKey)
			},
		},
		nil)
	go r.Start(ctx)

	if hs != nil {
		if err := hs(r); err != nil {
			r.Close()
			return nil, err
		}
	}

	// t.logger.Debug("=== DialRPC D ===", zap.String("peer", makeQKey(peer)))

	t.rpcMap.Store(rpcMapKey, r)

	return r, nil
}

func (t *QUIC) DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error) {
	if t.closed.Load() {
		return nil, transport.ErrClosed
	}

	q, stream, err := t.getS(ctx, peer, protocol.Stream_DIRECT)
	if err != nil {
		return nil, err
	}

	t.Logger.Debug("Created new Direct Stream",
		zap.String("remote", q.RemoteAddr().String()),
		zap.String("local", q.LocalAddr().String()))

	return WrapQuicConnection(stream, q), nil
}

func (t *QUIC) RPC() <-chan *transport.StreamDelegate {
	return t.rpcChan
}

func (t *QUIC) Direct() <-chan *transport.StreamDelegate {
	return t.directChan
}

func (t *QUIC) SupportDatagram() bool {
	return quicConfig.EnableDatagrams
}

func (t *QUIC) ReceiveDatagram() <-chan *transport.DatagramDelegate {
	return t.dgramChan
}

func (t *QUIC) SendDatagram(peer *protocol.Node, buf []byte) error {
	qKey := makeQKey(peer)
	if r, ok := t.qMap.Load(qKey); ok {
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

func (t *QUIC) reuseConnection(ctx context.Context, q quic.EarlyConnection, s quic.Stream, dir string) (*nodeConnection, bool, error) {
	rr := &protocol.Connection{
		Identity: t.Endpoint,
	}

	s.SetDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err := rpcSpec.Send(s, rr)
	if err != nil {
		return nil, false, err
	}
	rr.Reset()
	err = rpcSpec.Receive(s, rr)
	if err != nil {
		return nil, false, err
	}
	s.SetDeadline(time.Time{})

	if t.Endpoint.GetId() == rr.GetIdentity().GetId() {
		t.Logger.Debug("connecting to ourself, skipping connection reuse", zap.String("direction", dir))
		return &nodeConnection{
			peer: t.Endpoint,
			quic: q,
		}, false, nil
	}

	rKey := makeQKey(rr.GetIdentity())

	unlock := t.qMu.Lock(rKey)
	defer func() {
		unlock()
	}()

	sQ, loaded := t.qMap.LoadOrStoreLazy(rKey, func() *nodeConnection {
		return &nodeConnection{
			peer: rr.GetIdentity(),
			quic: q,
		}
	})
	if loaded {
		t.Logger.Debug("reusing quic connection", zap.String("direction", dir), zap.String("key", rKey))
		q.CloseWithError(0, "A previous connection was reused")
		return sQ, true, nil
	} else {
		t.Logger.Debug("saving quic connection for reuse", zap.String("direction", dir), zap.String("key", rKey))
	}

	return &nodeConnection{
		peer: rr.GetIdentity(),
		quic: q,
	}, false, nil
}

func (t *QUIC) handleIncoming(ctx context.Context, q quic.EarlyConnection) (quic.EarlyConnection, error) {
	openCtx, openCancel := context.WithTimeout(ctx, quicConfig.HandshakeIdleTimeout)
	defer openCancel()

	stream, err := q.AcceptStream(openCtx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	c, reused, err := t.reuseConnection(ctx, q, stream, "incoming")
	if err != nil {
		return nil, err
	}

	if !reused {
		t.handlePeer(ctx, c.quic, c.peer, "incoming")
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

	c, reused, err := t.reuseConnection(ctx, q, stream, "outgoing")
	if err != nil {
		return nil, err
	}

	if !reused && c.peer.GetId() != t.Endpoint.GetId() {
		t.handlePeer(ctx, c.quic, c.peer, "outgoing")
	}

	return c.quic, nil
}

func (t *QUIC) handlePeer(ctx context.Context, q quic.EarlyConnection, peer *protocol.Node, dir string) {
	t.Logger.Debug("Starting goroutines to handle streams and datagrams", zap.String("direction", dir), zap.String("key", makeQKey(peer)))
	go t.handleConnection(ctx, q, peer)
	go t.handleDatagram(ctx, q, peer)
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

func (t *QUIC) handleDatagram(ctx context.Context, q quic.Connection, peer *protocol.Node) {
	logger := t.Logger.With(zap.String("endpoint", q.RemoteAddr().String()))
	for {
		b, err := q.ReceiveMessage()
		if err != nil {
			// logger.Error("receiving datagram", zap.Error(err))
			return
		}
		data := &protocol.Datagram{}
		if err := data.UnmarshalVT(b); err != nil {
			logger.Error("decoding datagram to proto", zap.Error(err))
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
			// t.Logger.Error("Error accepting new stream from peer", zap.String("peer", peer.String()), zap.String("remote", q.RemoteAddr().String()), zap.Error(err))
			return
		}
		go t.streamRouter(q, stream, peer)
	}
}

func (t *QUIC) streamRouter(q quic.Connection, stream quic.Stream, peer *protocol.Node) {
	var err error
	defer func() {
		if err != nil {
			t.Logger.Error("handshake on new stream", zap.Error(err))
			stream.Close()
			return
		}
	}()

	rr := &protocol.Stream{}
	stream.SetDeadline(time.Now().Add(quicConfig.HandshakeIdleTimeout))
	err = rpcSpec.Receive(stream, rr)
	if err != nil {
		return
	}
	stream.SetDeadline(time.Time{})

	switch rr.GetType() {
	case protocol.Stream_RPC:
		t.rpcChan <- &transport.StreamDelegate{
			Connection: WrapQuicConnection(stream, q),
			Identity:   peer,
		}
	case protocol.Stream_DIRECT:
		t.directChan <- &transport.StreamDelegate{
			Connection: WrapQuicConnection(stream, q),
			Identity:   peer,
		}
	default:
		err = fmt.Errorf("unknown stream type: %s", rr.GetType())
	}
}

func (t *QUIC) Stop() {
	if !t.closed.CompareAndSwap(false, true) {
		return
	}
	t.started.Store(false)
	t.qMap.Range(func(key string, value *nodeConnection) bool {
		value.quic.CloseWithError(0, "Transport closed")
		return true
	})
}
