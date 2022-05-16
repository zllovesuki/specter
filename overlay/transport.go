package overlay

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"specter/rpc"
	"specter/spec/protocol"
	rpcSpec "specter/spec/rpc"
	"specter/spec/transport"

	"github.com/lucas-clemente/quic-go"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var (
	ErrClosed   = fmt.Errorf("transport is already closed")
	ErrNoDirect = fmt.Errorf("cannot open direct quic connection without address")
)

var _ transport.Transport = (*QUIC)(nil)

func NewQUIC(logger *zap.Logger, self *protocol.Node, serverTLS *tls.Config, clientTLS *tls.Config) *QUIC {
	return &QUIC{
		logger: logger,
		self:   self,

		rpcChan:    make(chan *transport.Delegate, 1),
		directChan: make(chan *transport.Delegate, 1),
		dgramChan:  make(chan *transport.DatagramDelegate, 32),

		server: serverTLS,
		client: clientTLS,

		closed: atomic.NewBool(false),
	}
}

func makeKey(peer *protocol.Node) string {
	qMapKey := peer.GetAddress() + "/"
	if peer.GetUnknown() {
		qMapKey = qMapKey + "-1"
	} else {
		qMapKey = qMapKey + strconv.FormatUint(peer.GetId(), 10)
	}
	return qMapKey
}

func (t *QUIC) getQ(ctx context.Context, peer *protocol.Node) (quic.Connection, error) {
	qKey := makeKey(peer)

	rUnlock := t.qMu.RLock(qKey)
	if sQ, ok := t.qMap.Load(qKey); ok {
		rUnlock()
		t.logger.Debug("Reusing quic connection from reuseMap", zap.String("key", qKey))
		return sQ.(*nodeConnection).quic, nil
	}
	rUnlock()

	if peer.GetAddress() == "" {
		return nil, ErrNoDirect
	}

	t.logger.Debug("Creating new QUIC connection", zap.Any("peer", peer))

	dialCtx, dialCancel := context.WithTimeout(ctx, time.Second)
	defer dialCancel()

	q, err := quic.DialAddrContext(dialCtx, peer.GetAddress(), t.client, quicConfig)
	if err != nil {
		return nil, err
	}

	q, err = t.handleOutgoing(ctx, q)
	if err != nil {
		return nil, err
	}
	return q, nil
}

func (t *QUIC) getS(ctx context.Context, peer *protocol.Node, sType protocol.Stream_Type) (q quic.Connection, stream quic.Stream, err error) {
	defer func() {
		if err != nil {
			t.logger.Error("Dialing new stream", zap.Error(err), zap.String("type", sType.String()), zap.String("addr", peer.GetAddress()))
		}
	}()

	q, err = t.getQ(ctx, peer)
	if err != nil {
		return nil, nil, err
	}

	openCtx, openCancel := context.WithTimeout(ctx, time.Second)
	defer openCancel()

	stream, err = q.OpenStreamSync(openCtx)
	if err != nil {
		return nil, nil, err
	}

	rr := &protocol.Stream{
		Type: sType,
	}
	stream.SetDeadline(time.Now().Add(time.Second))
	err = rpc.Send(stream, rr)
	if err != nil {
		return
	}
	stream.SetDeadline(time.Time{})

	return q, stream, nil
}

func (t *QUIC) Identity() *protocol.Node {
	return t.self
}

func (t *QUIC) DialRPC(ctx context.Context, peer *protocol.Node, hs rpcSpec.RPCHandshakeFunc) (rpcSpec.RPC, error) {
	if t.closed.Load() {
		return nil, ErrClosed
	}

	rpcMapKey := peer.GetAddress()

	rUnlock := t.rpcMu.RLock(rpcMapKey)
	if r, ok := t.rpcMap.Load(rpcMapKey); ok {
		rUnlock()
		return r.(rpcSpec.RPC), nil
	}
	rUnlock()

	t.logger.Debug("=== DialRPC B ===", zap.String("peer", makeKey(peer)))

	unlock := t.rpcMu.Lock(rpcMapKey)
	defer unlock()

	if r, ok := t.rpcMap.Load(rpcMapKey); ok {
		return r.(rpcSpec.RPC), nil
	}

	t.logger.Debug("=== DialRPC C ===", zap.String("peer", makeKey(peer)))

	q, stream, err := t.getS(ctx, peer, protocol.Stream_RPC)
	if err != nil {
		return nil, err
	}

	t.logger.Debug("Created new RPC Stream",
		zap.Any("peer", peer),
		zap.String("remote", q.RemoteAddr().String()),
		zap.String("local", q.LocalAddr().String()))

	r := rpc.NewRPC(
		t.logger.With(
			zap.Any("peer", peer),
			zap.String("remote", q.RemoteAddr().String()),
			zap.String("local", q.LocalAddr().String()),
			zap.String("pov", "transport_dial")),
		stream,
		nil)
	go r.Start(ctx)

	if hs != nil {
		if err := hs(r); err != nil {
			r.Close()
			return nil, err
		}
	}

	t.logger.Debug("=== DialRPC D ===", zap.String("peer", makeKey(peer)))

	t.rpcMap.Store(rpcMapKey, r)

	return r, nil
}

func (t *QUIC) DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error) {
	if t.closed.Load() {
		return nil, ErrClosed
	}

	q, stream, err := t.getS(ctx, peer, protocol.Stream_DIRECT)
	if err != nil {
		return nil, err
	}

	t.logger.Debug("Created new Direct Stream",
		zap.String("remote", q.RemoteAddr().String()),
		zap.String("local", q.LocalAddr().String()))

	return w(q, stream), nil
}

func (t *QUIC) RPC() <-chan *transport.Delegate {
	return t.rpcChan
}

func (t *QUIC) Direct() <-chan *transport.Delegate {
	return t.directChan
}

func (t *QUIC) SupportDatagram() bool {
	return quicConfig.EnableDatagrams
}

func (t *QUIC) ReceiveDatagram() <-chan *transport.DatagramDelegate {
	return t.dgramChan
}

func (t *QUIC) SendDatagram(peer *protocol.Node, buf []byte) error {
	qKey := makeKey(peer)
	if r, ok := t.qMap.Load(qKey); ok {
		data := &protocol.Datagram{
			Type: protocol.Datagram_DATA,
			Data: buf,
		}
		b, err := proto.Marshal(data)
		if err != nil {
			return err
		}
		return r.(*nodeConnection).quic.SendMessage(b)
	}
	return fmt.Errorf("peer %s is not registered in transport", qKey)
}

func (t *QUIC) reuseConnection(ctx context.Context, q quic.Connection, s quic.Stream, dir string) (quic.Connection, *protocol.Node, bool, error) {
	rr := &protocol.Connection{
		Identity: t.self,
	}

	s.SetDeadline(time.Now().Add(time.Second))
	err := rpc.Send(s, rr)
	if err != nil {
		return nil, nil, false, err
	}
	err = rpc.Receive(s, rr)
	if err != nil {
		return nil, nil, false, err
	}
	s.SetDeadline(time.Time{})

	if t.self.GetId() == rr.GetIdentity().GetId() {
		t.logger.Debug("connecting to ourself, skipping connection reuse", zap.String("direction", dir))
		return q, t.self, false, nil
	}

	rKey := makeKey(rr.GetIdentity())
	cached := &nodeConnection{
		peer: rr.GetIdentity(),
		quic: q,
	}

	unlock := t.qMu.Lock(rKey)
	defer unlock()

	sQ, loaded := t.qMap.LoadOrStore(rKey, cached)
	if loaded {
		t.logger.Debug("reusing quic connection", zap.String("direction", dir), zap.String("key", rKey))
		q.CloseWithError(0, "A previous connection was reused")
		return sQ.(*nodeConnection).quic, rr.GetIdentity(), true, nil
	} else {
		t.logger.Debug("saving quic connection for reuse", zap.String("direction", dir), zap.String("key", rKey))
	}

	return q, rr.GetIdentity(), false, nil
}

func (t *QUIC) handleIncoming(ctx context.Context, q quic.Connection) (quic.Connection, error) {
	openCtx, openCancel := context.WithTimeout(ctx, time.Second)
	defer openCancel()

	stream, err := q.OpenStreamSync(openCtx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	newQ, peer, reused, err := t.reuseConnection(ctx, q, stream, "incoming")
	if err != nil {
		return nil, err
	}

	if !reused {
		t.handlePeer(ctx, newQ, peer, "incoming")
	}

	return newQ, nil
}

func (t *QUIC) handleOutgoing(ctx context.Context, q quic.Connection) (quic.Connection, error) {
	openCtx, openCancel := context.WithTimeout(ctx, time.Second)
	defer openCancel()

	stream, err := q.AcceptStream(openCtx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	newQ, peer, reused, err := t.reuseConnection(ctx, q, stream, "outgoing")
	if err != nil {
		return nil, err
	}

	if !reused && peer.GetId() != t.self.GetId() {
		t.handlePeer(ctx, newQ, peer, "outgoing")
	}

	return newQ, nil
}

func (t *QUIC) handlePeer(ctx context.Context, q quic.Connection, peer *protocol.Node, dir string) {
	t.logger.Debug("Starting goroutines to handle incoming streams and datagrams", zap.String("direction", dir), zap.String("key", makeKey(peer)))
	go t.handleConnection(ctx, q, peer)
	go t.handleDatagram(ctx, q, peer)
}

func (t *QUIC) Accept(ctx context.Context) error {
	l, err := quic.ListenAddr(t.self.GetAddress(), t.server, quicConfig)
	if err != nil {
		return err
	}

	t.logger.Info("Accepting connections", zap.String("listen", t.self.GetAddress()))

	go t.reaper(ctx)

	for {
		q, err := l.Accept(ctx)
		if err != nil {
			return err
		}
		go func(prev quic.Connection) {
			if _, err := t.handleIncoming(ctx, prev); err != nil {
				t.logger.Error("incoming connection reuse error", zap.Error(err))
			}
		}(q)
	}
}

func (t *QUIC) handleDatagram(ctx context.Context, q quic.Connection, peer *protocol.Node) {
	logger := t.logger.With(zap.String("endpoint", q.RemoteAddr().String()))
	for {
		b, err := q.ReceiveMessage()
		if err != nil {
			// logger.Error("receiving datagram", zap.Error(err))
			return
		}
		data := &protocol.Datagram{}
		if err := proto.Unmarshal(b, data); err != nil {
			logger.Error("decoding datagram to proto", zap.Error(err))
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
			// t.logger.Error("accepting new stream", zap.Error(err), zap.String("remote", q.RemoteAddr().String()))
			return
		}
		go t.streamRouter(q, stream, peer)
	}
}

func (t *QUIC) streamRouter(q quic.Connection, stream quic.Stream, peer *protocol.Node) {
	var err error
	defer func() {
		if err != nil {
			t.logger.Error("handshake on new stream", zap.Error(err))
			stream.Close()
			return
		}
	}()

	rr := &protocol.Stream{}
	stream.SetDeadline(time.Now().Add(time.Second))
	err = rpc.Receive(stream, rr)
	if err != nil {
		return
	}
	stream.SetDeadline(time.Time{})

	switch rr.GetType() {
	case protocol.Stream_RPC:
		t.rpcChan <- &transport.Delegate{
			Connection: w(q, stream),
			Identity:   peer,
		}
	case protocol.Stream_DIRECT:
		t.directChan <- &transport.Delegate{
			Connection: w(q, stream),
			Identity:   peer,
		}
	default:
		err = fmt.Errorf("unknown stream type: %s", rr.GetType())
	}
}

func (t *QUIC) Stop() {
	if !t.closed.CAS(false, true) {
		return
	}
}

func w(q quic.Connection, s quic.Stream) *quicConn {
	return &quicConn{
		Stream: s,
		q:      q,
	}
}
