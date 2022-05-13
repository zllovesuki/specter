package overlay

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"time"

	"specter/spec/protocol"

	"github.com/lucas-clemente/quic-go"
	"go.uber.org/zap"
)

type Transport struct {
	logger *zap.Logger

	qMap map[string]quic.Connection
	qMu  sync.RWMutex

	rpcMap map[string]*RPC
	rpcMu  sync.RWMutex

	rpcChan    chan quic.Stream
	tunnelChan chan quic.Stream

	server *tls.Config
	client *tls.Config
}

func NewTransport(logger *zap.Logger, serverTLS *tls.Config, clientTLS *tls.Config) *Transport {
	return &Transport{
		logger:     logger,
		qMap:       make(map[string]quic.Connection),
		rpcMap:     make(map[string]*RPC),
		rpcChan:    make(chan quic.Stream, 1),
		tunnelChan: make(chan quic.Stream, 1),

		server: serverTLS,
		client: clientTLS,
	}
}

func (t *Transport) getQ(ctx context.Context, addr string) (quic.Connection, error) {
	t.qMu.RLock()
	if q, ok := t.qMap[addr]; ok {
		t.qMu.RUnlock()
		return q, nil
	}
	t.qMu.RUnlock()

	t.logger.Debug("Creating new QUIC connection", zap.String("addr", addr))

	dialCtx, dialCancel := context.WithTimeout(ctx, time.Second*3)
	defer dialCancel()

	t.qMu.Lock()
	q, err := quic.DialAddrContext(dialCtx, addr, t.client, quicConfig)
	if err != nil {
		t.qMu.Unlock()
		return nil, err
	}
	t.qMap[addr] = q
	t.qMu.Unlock()

	return q, nil
}

func (t *Transport) getS(ctx context.Context, addr string) (stream quic.Stream, err error) {
	defer func() {
		if err != nil {
			t.logger.Error("Dialing RPC stream", zap.Error(err), zap.String("addr", addr))
		}
		if stream != nil {
			stream.SetDeadline(time.Time{})
		}
	}()
	var q quic.Connection

	q, err = t.getQ(ctx, addr)
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
	}
	err = sendRPC(stream, rr)
	if err != nil {
		return
	}
	return stream, nil
}

func (t *Transport) DialRPC(ctx context.Context, addr string, hs func(*RPC) error) (*RPC, error) {
	t.rpcMu.RLock()
	if r, ok := t.rpcMap[addr]; ok {
		t.rpcMu.RUnlock()
		return r, nil
	}
	t.rpcMu.RUnlock()

	t.logger.Debug("Creating new RPC Stream", zap.String("addr", addr))

	t.rpcMu.Lock()
	defer t.rpcMu.Unlock()

	stream, err := t.getS(ctx, addr)
	if err != nil {
		return nil, err
	}

	r := NewRPC(stream, nil)
	go r.Start(ctx)

	if hs != nil {
		if err := hs(r); err != nil {
			r.Close()
			return nil, err
		}
	}

	t.rpcMap[addr] = r

	return r, nil
}

func (t *Transport) RPC() <-chan quic.Stream {
	return t.rpcChan
}

func (t *Transport) Tunnel() <-chan quic.Stream {
	return t.tunnelChan
}

func (t *Transport) Accept(ctx context.Context, identity *protocol.Node) error {
	l, err := quic.ListenAddr(identity.GetAddress(), t.server, quicConfig)
	if err != nil {
		return err
	}
	for {
		q, err := l.Accept(ctx)
		if err != nil {
			return err
		}
		go t.handleConnection(ctx, q)
	}
}

func (t *Transport) handleConnection(ctx context.Context, q quic.Connection) {
	for {
		stream, err := q.AcceptStream(ctx)
		if err != nil {
			return
		}
		go t.streamRouter(stream)
	}
}

func (t *Transport) streamRouter(stream quic.Stream) {
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
	err = receiveRPC(stream, rr)
	if err != nil {
		return
	}

	switch rr.GetType() {
	case protocol.Stream_RPC:
		t.rpcChan <- stream
	case protocol.Stream_TUNNEL:
		t.tunnelChan <- stream
	default:
		err = errors.New("wtf")
	}
}
