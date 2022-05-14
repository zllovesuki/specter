package overlay

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"time"

	"specter/rpc"
	"specter/spec"
	"specter/spec/protocol"
	"specter/spec/transport"

	"github.com/lucas-clemente/quic-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var _ transport.Transport = (*QUIC)(nil)

func NewQUIC(logger *zap.Logger, serverTLS *tls.Config, clientTLS *tls.Config) *QUIC {
	return &QUIC{
		logger:     logger,
		qMap:       make(map[string]*nodeConnection),
		rpcMap:     make(map[string]spec.RPC),
		rpcChan:    make(chan transport.Stream, 1),
		directChan: make(chan transport.Stream, 1),

		server: serverTLS,
		client: clientTLS,
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
	qMapKey := makeKey(peer)

	t.qMu.RLock()
	if q, ok := t.qMap[qMapKey]; ok {
		t.qMu.RUnlock()
		return q.quic, nil
	}
	t.qMu.RUnlock()

	t.qMu.Lock()
	defer t.qMu.Unlock()

	if q, ok := t.qMap[qMapKey]; ok {
		return q.quic, nil
	}

	t.logger.Debug("Creating new QUIC connection", zap.String("addr", peer.GetAddress()))

	dialCtx, dialCancel := context.WithTimeout(ctx, time.Second)
	defer dialCancel()
	q, err := quic.DialAddrContext(dialCtx, peer.GetAddress(), t.client, quicConfig)
	if err != nil {
		return nil, err
	}
	t.qMap[qMapKey] = &nodeConnection{
		peer: peer,
		quic: q,
	}

	return q, nil
}

func (t *QUIC) getS(ctx context.Context, peer *protocol.Node, sType protocol.Stream_Type) (q quic.Connection, stream quic.Stream, err error) {
	defer func() {
		if err != nil {
			t.logger.Error("Dialing new stream", zap.Error(err), zap.String("type", sType.String()), zap.String("addr", peer.GetAddress()))
		}
		if stream != nil {
			stream.SetDeadline(time.Time{})
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
	stream.SetDeadline(time.Now().Add(time.Second))

	rr := &protocol.Stream{
		Type: sType,
	}
	err = rpc.Send(stream, rr)
	if err != nil {
		return
	}
	return q, stream, nil
}

func (t *QUIC) DialRPC(ctx context.Context, peer *protocol.Node, hs spec.RPCHandshakeFunc) (spec.RPC, error) {
	rpcMapKey := peer.GetAddress()

	t.rpcMu.RLock()
	if r, ok := t.rpcMap[rpcMapKey]; ok {
		t.rpcMu.RUnlock()
		return r, nil
	}
	t.rpcMu.RUnlock()

	t.rpcMu.Lock()
	defer t.rpcMu.Unlock()

	if r, ok := t.rpcMap[rpcMapKey]; ok {
		return r, nil
	}

	t.logger.Debug("Creating new RPC Stream", zap.String("addr", peer.GetAddress()))

	_, stream, err := t.getS(ctx, peer, protocol.Stream_RPC)
	if err != nil {
		return nil, err
	}

	r := rpc.NewRPC(t.logger.With(zap.String("addr", peer.GetAddress()), zap.String("pov", "transport_dial")), stream, nil)
	go r.Start(ctx)

	if hs != nil {
		if err := hs(r); err != nil {
			r.Close()
			return nil, err
		}
	}

	t.rpcMap[rpcMapKey] = r

	return r, nil
}

func (t *QUIC) DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error) {
	t.logger.Debug("Creating new Tunnel Stream", zap.String("addr", peer.GetAddress()))

	q, stream, err := t.getS(ctx, peer, protocol.Stream_TUNNEL)
	if err != nil {
		return nil, err
	}

	return w(q, stream), nil
}

func (t *QUIC) RPC() <-chan transport.Stream {
	return t.rpcChan
}

func (t *QUIC) Direct() <-chan transport.Stream {
	return t.directChan
}

func (t *QUIC) Accept(ctx context.Context, identity *protocol.Node) error {
	l, err := quic.ListenAddr(identity.GetAddress(), t.server, quicConfig)
	if err != nil {
		return err
	}

	go t.reaper(ctx)

	for {
		q, err := l.Accept(ctx)
		if err != nil {
			return err
		}
		go t.handleConnection(ctx, q)
		go t.handleDatagram(ctx, q)
	}
}

func (t *QUIC) handleDatagram(ctx context.Context, q quic.Connection) {
	logger := t.logger.With(zap.String("endpoint", q.RemoteAddr().String()))
	for {
		b, err := q.ReceiveMessage()
		if err != nil {
			logger.Error("receiving datagram", zap.Error(err))
			return
		}
		data := &protocol.Datagram{}
		if err := proto.Unmarshal(b, data); err != nil {
			logger.Error("decoding datagram to proto", zap.Error(err))
		}
		// logger.Debug("received datagram", zap.String("data", data.String()))
	}
}

func (t *QUIC) handleConnection(ctx context.Context, q quic.Connection) {
	for {
		stream, err := q.AcceptStream(ctx)
		if err != nil {
			t.logger.Error("accepting new stream", zap.Error(err), zap.String("remote", q.RemoteAddr().String()))
			return
		}
		go t.streamRouter(q, stream)
	}
}

func (t *QUIC) streamRouter(q quic.Connection, stream quic.Stream) {
	var err error
	defer func() {
		if err != nil {
			t.logger.Error("Stream Handshake", zap.Error(err))
			stream.Close()
			return
		}
		stream.SetDeadline(time.Time{})
	}()

	stream.SetDeadline(time.Now().Add(time.Second))

	rr := &protocol.Stream{}
	err = rpc.Receive(stream, rr)
	if err != nil {
		return
	}

	switch rr.GetType() {
	case protocol.Stream_RPC:
		t.rpcChan <- transport.Stream{
			Connection: w(q, stream),
			Remote:     q.RemoteAddr(),
		}
	case protocol.Stream_TUNNEL:
		t.directChan <- transport.Stream{
			Connection: w(q, stream),
			Remote:     q.RemoteAddr(),
		}
	default:
		err = errors.New("wtf")
	}
}

func w(q quic.Connection, s quic.Stream) *quicConn {
	return &quicConn{
		Stream: s,
		q:      q,
	}
}
