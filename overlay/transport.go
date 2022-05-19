package overlay

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/zllovesuki/specter/rpc"
	"github.com/zllovesuki/specter/spec/concurrent"
	"github.com/zllovesuki/specter/spec/protocol"
	rpcSpec "github.com/zllovesuki/specter/spec/rpc"
	"github.com/zllovesuki/specter/spec/transport"

	"github.com/lucas-clemente/quic-go"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var (
	ErrClosed   = fmt.Errorf("transport is already closed")
	ErrNoDirect = fmt.Errorf("cannot open direct quic connection without address")
)

var _ transport.Transport = (*QUIC)(nil)

func NewQUIC(conf TransportConfig) *QUIC {
	return &QUIC{
		TransportConfig: conf,

		qMap:   skipmap.NewString(),
		qMu:    concurrent.NewKeyedRWMutex(),
		rpcMap: skipmap.NewString(),
		rpcMu:  concurrent.NewKeyedRWMutex(),

		rpcChan:    make(chan *transport.StreamDelegate),
		directChan: make(chan *transport.StreamDelegate),
		dgramChan:  make(chan *transport.DatagramDelegate, 32),

		estChan: make(chan *protocol.Node),
		desChan: make(chan *protocol.Node),

		started: atomic.NewBool(false),
		closed:  atomic.NewBool(false),
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

func (t *QUIC) getQ(ctx context.Context, peer *protocol.Node) (quic.Connection, error) {
	qKey := makeQKey(peer)

	rUnlock := t.qMu.RLock(qKey)
	if sQ, ok := t.qMap.Load(qKey); ok {
		rUnlock()
		t.Logger.Debug("Reusing quic connection from reuseMap", zap.String("key", qKey))
		return sQ.(*nodeConnection).quic, nil
	}
	rUnlock()

	if peer.GetAddress() == "" {
		return nil, ErrNoDirect
	}

	t.Logger.Debug("Creating new QUIC connection", zap.Any("peer", peer))

	dialCtx, dialCancel := context.WithTimeout(ctx, time.Second)
	defer dialCancel()

	q, err := quic.DialAddrContext(dialCtx, peer.GetAddress(), t.ClientTLS, quicConfig)
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

func (t *QUIC) getS(ctx context.Context, peer *protocol.Node, sType protocol.Stream_Type) (q quic.Connection, stream quic.Stream, err error) {
	defer func() {
		if err != nil {
			t.Logger.Error("Dialing new stream", zap.Error(err), zap.String("type", sType.String()), zap.String("addr", peer.GetAddress()))
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
	return t.Endpoint
}

func (t *QUIC) DialRPC(ctx context.Context, peer *protocol.Node, hs rpcSpec.RPCHandshakeFunc) (rpcSpec.RPC, error) {
	if t.closed.Load() {
		return nil, ErrClosed
	}

	rpcMapKey := makeSKey(peer)

	rUnlock := t.rpcMu.RLock(rpcMapKey)
	if r, ok := t.rpcMap.Load(rpcMapKey); ok {
		rUnlock()
		return r.(rpcSpec.RPC), nil
	}
	rUnlock()

	// t.logger.Debug("=== DialRPC B ===", zap.String("peer", makeQKey(peer)))

	unlock := t.rpcMu.Lock(rpcMapKey)
	defer unlock()

	if r, ok := t.rpcMap.Load(rpcMapKey); ok {
		return r.(rpcSpec.RPC), nil
	}

	// t.logger.Debug("=== DialRPC C ===", zap.String("peer", makeQKey(peer)))

	q, stream, err := t.getS(ctx, peer, protocol.Stream_RPC)
	if err != nil {
		return nil, err
	}

	l := t.Logger.With(
		zap.Any("peer", peer),
		zap.String("remote", q.RemoteAddr().String()),
		zap.String("local", q.LocalAddr().String()))

	l.Debug("Created new RPC Stream")

	r := rpc.NewRPC(
		l.With(zap.String("pov", "transport_dial")),
		stream,
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
		return nil, ErrClosed
	}

	q, stream, err := t.getS(ctx, peer, protocol.Stream_DIRECT)
	if err != nil {
		return nil, err
	}

	t.Logger.Debug("Created new Direct Stream",
		zap.String("remote", q.RemoteAddr().String()),
		zap.String("local", q.LocalAddr().String()))

	return w(q, stream), nil
}

func (t *QUIC) RPC() <-chan *transport.StreamDelegate {
	return t.rpcChan
}

func (t *QUIC) Direct() <-chan *transport.StreamDelegate {
	return t.directChan
}

func (t *QUIC) TransportEstablished() <-chan *protocol.Node {
	return t.estChan
}

func (t *QUIC) TransportDestroyed() <-chan *protocol.Node {
	return t.desChan
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
		Identity: t.Endpoint,
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

	if t.Endpoint.GetId() == rr.GetIdentity().GetId() {
		t.Logger.Debug("connecting to ourself, skipping connection reuse", zap.String("direction", dir))
		return q, t.Endpoint, false, nil
	}

	rKey := makeQKey(rr.GetIdentity())

	unlock := t.qMu.Lock(rKey)
	defer unlock()

	sQ, loaded := t.qMap.LoadOrStoreLazy(rKey, func() interface{} {
		return &nodeConnection{
			peer: rr.GetIdentity(),
			quic: q,
		}
	})
	if loaded {
		t.Logger.Debug("reusing quic connection", zap.String("direction", dir), zap.String("key", rKey))
		q.CloseWithError(0, "A previous connection was reused")
		return sQ.(*nodeConnection).quic, rr.GetIdentity(), true, nil
	} else {
		t.Logger.Debug("saving quic connection for reuse", zap.String("direction", dir), zap.String("key", rKey))
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
		select {
		case t.estChan <- peer:
		default:
		}
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

	if !reused && peer.GetId() != t.Endpoint.GetId() {
		select {
		case t.estChan <- peer:
		default:
		}
		t.handlePeer(ctx, newQ, peer, "outgoing")
	}

	return newQ, nil
}

func (t *QUIC) handlePeer(ctx context.Context, q quic.Connection, peer *protocol.Node, dir string) {
	t.Logger.Debug("Starting goroutines to handle incoming streams and datagrams", zap.String("direction", dir), zap.String("key", makeQKey(peer)))
	go t.handleConnection(ctx, q, peer)
	go t.handleDatagram(ctx, q, peer)
}

func (t *QUIC) background(ctx context.Context) {
	if !t.started.CAS(false, true) {
		return
	}
	go t.reaper(ctx)
}

func (t *QUIC) Accept(ctx context.Context) error {
	l, err := quic.ListenAddr(t.Endpoint.GetAddress(), t.ServerTLS, quicConfig)
	if err != nil {
		return err
	}

	t.background(ctx)

	t.Logger.Info("Accepting connections", zap.String("listen", t.Endpoint.GetAddress()))

	for {
		q, err := l.Accept(ctx)
		if err != nil {
			return err
		}
		go func(prev quic.Connection) {
			if _, err := t.handleIncoming(ctx, prev); err != nil {
				t.Logger.Error("incoming connection reuse error", zap.Error(err))
			}
		}(q)
	}
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
			t.Logger.Error("handshake on new stream", zap.Error(err))
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
		t.rpcChan <- &transport.StreamDelegate{
			Connection: w(q, stream),
			Identity:   peer,
		}
	case protocol.Stream_DIRECT:
		t.directChan <- &transport.StreamDelegate{
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
	t.started.Store(false)
	t.qMap.Range(func(key string, value interface{}) bool {
		n := value.(*nodeConnection)
		n.quic.CloseWithError(0, "Transport closed")
		return true
	})
}

func w(q quic.Connection, s quic.Stream) *quicConn {
	return &quicConn{
		Stream: s,
		q:      q,
	}
}
