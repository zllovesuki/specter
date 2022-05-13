package overlay

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
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

	peerRpcChan   chan quic.Stream
	clientRpcChan chan quic.Stream
	tunnelChan    chan quic.Stream

	server *tls.Config
	client *tls.Config
}

func NewTransport(logger *zap.Logger, serverTLS *tls.Config, clientTLS *tls.Config) *Transport {
	return &Transport{
		logger:        logger,
		qMap:          make(map[string]quic.Connection),
		rpcMap:        make(map[string]*RPC),
		peerRpcChan:   make(chan quic.Stream, 1),
		clientRpcChan: make(chan quic.Stream, 1),
		tunnelChan:    make(chan quic.Stream, 1),

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

	t.qMu.Lock()
	defer t.qMu.Unlock()

	if q, ok := t.qMap[addr]; ok {
		return q, nil
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, time.Second)
	defer dialCancel()
	q, err := quic.DialAddrEarlyContext(dialCtx, addr, t.client, quicConfig)
	if err != nil {
		return nil, err
	}
	t.qMap[addr] = q

	return q, nil
}

func (t *Transport) getS(ctx context.Context, addr string, rpcType protocol.Stream_Procedure) (stream quic.Stream, err error) {
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
		Rpc:  rpcType,
	}
	err = sendRPC(stream, rr)
	if err != nil {
		return
	}
	return stream, nil
}

func (t *Transport) DialRPC(ctx context.Context, addr string, rpcType protocol.Stream_Procedure, hs func(*RPC) error) (*RPC, error) {
	rpcMapKey := rpcType.String() + "/" + addr

	t.rpcMu.RLock()
	if r, ok := t.rpcMap[rpcMapKey]; ok {
		t.rpcMu.RUnlock()
		return r, nil
	}
	t.rpcMu.RUnlock()

	t.logger.Debug("Creating new RPC Stream", zap.String("addr", addr))

	t.rpcMu.Lock()
	defer t.rpcMu.Unlock()

	if r, ok := t.rpcMap[rpcMapKey]; ok {
		return r, nil
	}

	stream, err := t.getS(ctx, addr, rpcType)
	if err != nil {
		return nil, err
	}

	r := NewRPC(t.logger.With(zap.String("addr", addr), zap.String("pov", "transport_dial")), stream, nil)
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

func (t *Transport) PeerRPC() <-chan quic.Stream {
	return t.peerRpcChan
}

func (t *Transport) ClientRPC() <-chan quic.Stream {
	return t.clientRpcChan
}

func (t *Transport) Tunnel() <-chan quic.Stream {
	return t.tunnelChan
}

func (t *Transport) reaper(ctx context.Context) {
	timer := time.NewTimer(quicConfig.MaxIdleTimeout)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			rL := make([]string, 0)
			rR := make([]*RPC, 0)
			t.rpcMu.RLock()
			for k, v := range t.rpcMap {
				fmt.Printf("now: %v, last %v, since: %v\n", time.Now(), v.last.Load(), time.Since((v.last.Load())))
				if time.Since(v.last.Load()) >= quicConfig.MaxIdleTimeout {
					rL = append(rL, k)
					rR = append(rR, v)
				}
			}
			t.rpcMu.RUnlock()

			for i, v := range rR {
				t.logger.Debug("Reaping RPC channel", zap.String("addr", rL[i]))
				v.Close()
				t.rpcMu.Lock()
				t.rpcMap[rL[i]] = nil
				t.rpcMu.Unlock()
			}

			timer.Reset(quicConfig.MaxIdleTimeout)
		}
	}
}

func (t *Transport) Accept(ctx context.Context, identity *protocol.Node) error {
	l, err := quic.ListenAddrEarly(identity.GetAddress(), t.server, quicConfig)
	if err != nil {
		return err
	}

	// go t.reaper(ctx)

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
		switch rr.GetRpc() {
		case protocol.Stream_PEER:
			t.peerRpcChan <- stream
		case protocol.Stream_CLIENT:
			t.clientRpcChan <- stream
		default:
			err = errors.New("wtf")
		}
	case protocol.Stream_TUNNEL:
		t.tunnelChan <- stream
	default:
		err = errors.New("wtf")
	}
}
