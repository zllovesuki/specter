package overlay

import (
	"context"
	"crypto/tls"
	"errors"
	"strconv"
	"time"

	"specter/rpc"
	"specter/spec/protocol"

	"github.com/lucas-clemente/quic-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func NewTransport(logger *zap.Logger, serverTLS *tls.Config, clientTLS *tls.Config) *Transport {
	return &Transport{
		logger:        logger,
		qMap:          make(map[string]*nodeConnection),
		rpcMap:        make(map[string]*rpc.RPC),
		peerRpcChan:   make(chan Stream, 1),
		clientRpcChan: make(chan Stream, 1),
		tunnelChan:    make(chan Stream, 1),

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

func (t *Transport) getQ(ctx context.Context, peer *protocol.Node) (quic.Connection, error) {
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
	q, err := quic.DialAddrEarlyContext(dialCtx, peer.GetAddress(), t.client, quicConfig)
	if err != nil {
		return nil, err
	}
	t.qMap[qMapKey] = &nodeConnection{
		peer: peer,
		quic: q,
	}

	return q, nil
}

func (t *Transport) getS(ctx context.Context, peer *protocol.Node, rpcType protocol.Stream_Procedure) (stream quic.Stream, err error) {
	defer func() {
		if err != nil {
			t.logger.Error("Dialing RPC stream", zap.Error(err), zap.String("addr", peer.GetAddress()))
		}
		if stream != nil {
			stream.SetDeadline(time.Time{})
		}
	}()
	var q quic.Connection

	q, err = t.getQ(ctx, peer)
	if err != nil {
		return nil, err
	}

	openCtx, openCancel := context.WithTimeout(ctx, time.Second)
	defer openCancel()

	stream, err = q.OpenStreamSync(openCtx)
	if err != nil {
		return nil, err
	}
	stream.SetDeadline(time.Now().Add(time.Second))

	rr := &protocol.Stream{
		Type: protocol.Stream_RPC,
		Rpc:  rpcType,
	}
	err = rpc.Send(stream, rr)
	if err != nil {
		return
	}
	return stream, nil
}

func (t *Transport) DialRPC(ctx context.Context, peer *protocol.Node, rpcType protocol.Stream_Procedure, hs RPCHandshakeFunc) (*rpc.RPC, error) {
	rpcMapKey := peer.GetAddress() + "/" + rpcType.String()

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

	stream, err := t.getS(ctx, peer, rpcType)
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

func (t *Transport) PeerRPC() <-chan Stream {
	return t.peerRpcChan
}

func (t *Transport) ClientRPC() <-chan Stream {
	return t.clientRpcChan
}

func (t *Transport) Tunnel() <-chan Stream {
	return t.tunnelChan
}

func (t *Transport) Accept(ctx context.Context, identity *protocol.Node) error {
	l, err := quic.ListenAddrEarly(identity.GetAddress(), t.server, quicConfig)
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

func (t *Transport) handleDatagram(ctx context.Context, q quic.Connection) {
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

func (t *Transport) handleConnection(ctx context.Context, q quic.Connection) {
	for {
		stream, err := q.AcceptStream(ctx)
		if err != nil {
			t.logger.Error("accepting new stream", zap.Error(err), zap.String("remote", q.RemoteAddr().String()))
			return
		}
		go t.streamRouter(q, stream)
	}
}

func (t *Transport) streamRouter(q quic.Connection, stream quic.Stream) {
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
		switch rr.GetRpc() {
		case protocol.Stream_PEER:
			t.peerRpcChan <- Stream{
				Connection: stream,
				Remote:     q.RemoteAddr(),
			}
		case protocol.Stream_CLIENT:
			t.clientRpcChan <- Stream{
				Connection: stream,
				Remote:     q.RemoteAddr(),
			}
		default:
			err = errors.New("wtf")
		}
	case protocol.Stream_TUNNEL:
		t.tunnelChan <- Stream{
			Connection: stream,
			Remote:     q.RemoteAddr(),
		}
	default:
		err = errors.New("wtf")
	}
}
