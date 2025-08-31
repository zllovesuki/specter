package overlay

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"time"

	"go.miragespace.co/specter/spec/transport/q"
	"go.miragespace.co/specter/util/acceptor"

	"github.com/quic-go/quic-go"
)

type ALPNMux struct {
	listener *quic.Listener
	mux      sync.Map // map[string]*protoCfg
}

type protoCfg struct {
	acceptor *acceptor.HTTP3Acceptor
	tls      *tls.Config
}

func NewMux(tr *quic.Transport) (*ALPNMux, error) {
	a := &ALPNMux{}
	q, err := tr.Listen(&tls.Config{
		GetConfigForClient: a.getConfigForClient,
	}, quicConfig)
	if err != nil {
		return nil, err
	}
	a.listener = q
	return a, nil
}

func (a *ALPNMux) getConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	var baseCfg *tls.Config
	var xCfg *tls.Config
	var selected string
	var found bool

	for _, propose := range hello.SupportedProtos {
		cfg, ok := a.mux.Load(propose)
		if ok {
			found = true
			selected = propose
			baseCfg = cfg.(*protoCfg).tls
			break
		}
	}

	if found {
		xCfg = baseCfg.Clone()
		xCfg.NextProtos = []string{selected}
		return xCfg, nil
	}

	return nil, errors.New("cipher: no mutually supported protocols")
}

func (a *ALPNMux) With(baseCfg *tls.Config, protos ...string) q.Listener {
	cfg := &protoCfg{
		acceptor: acceptor.NewH3Acceptor(a.listener),
		tls:      baseCfg,
	}
	for _, proto := range protos {
		a.mux.Store(proto, cfg)
	}
	return cfg.acceptor
}

func (a *ALPNMux) Accept(ctx context.Context) {
	for {
		conn, err := a.listener.Accept(ctx)
		if err != nil {
			return
		}
		go a.handleConnection(ctx, conn)
	}
}

func (a *ALPNMux) Close() {
	a.mux.Range(func(_, value any) bool {
		cfg := value.(*protoCfg)
		cfg.acceptor.Close()
		return true
	})
	a.listener.Close()
}

func (a *ALPNMux) handleConnection(ctx context.Context, conn *quic.Conn) {
	select {
	case <-time.After(quicConfig.HandshakeIdleTimeout):
		conn.CloseWithError(401, "Gone")
		return
	case <-ctx.Done():
		conn.CloseWithError(401, "Gone")
		return
	case <-conn.HandshakeComplete():
	}

	cs := conn.ConnectionState().TLS

	cfg, ok := a.mux.Load(cs.NegotiatedProtocol)
	if !ok {
		conn.CloseWithError(404, "Unsupported protocol")
		return
	}
	cfg.(*protoCfg).acceptor.Handle(conn)
}
