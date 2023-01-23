package overlay

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"kon.nect.sh/specter/util/acceptor"

	"github.com/quic-go/quic-go"
	"github.com/zhangyunhao116/skipmap"
)

type ALPNMux struct {
	listener quic.EarlyListener
	mux      *skipmap.StringMap[*protoCfg]
}

type protoCfg struct {
	acceptor *acceptor.HTTP3Acceptor
	tls      *tls.Config
}

func NewMux(listener net.PacketConn) (*ALPNMux, error) {
	a := &ALPNMux{
		mux: skipmap.NewString[*protoCfg](),
	}
	q, err := quic.ListenEarly(listener, &tls.Config{
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

	a.mux.Range(func(proto string, cfg *protoCfg) bool {
		for _, cP := range hello.SupportedProtos {
			if proto == cP {
				found = true
				selected = proto
				baseCfg = cfg.tls
				return false
			}
		}
		return true
	})

	if found {
		xCfg = baseCfg.Clone()
		xCfg.NextProtos = []string{selected}
		return xCfg, nil
	}

	return nil, errors.New("cipher: no mutually supported protocols")
}

func (a *ALPNMux) With(baseCfg *tls.Config, protos ...string) quic.EarlyListener {
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
	a.mux.Range(func(_ string, cfg *protoCfg) bool {
		cfg.acceptor.Close()
		return true
	})
	a.listener.Close()
}

func (a *ALPNMux) handleConnection(ctx context.Context, conn quic.EarlyConnection) {
	hsCtx := conn.HandshakeComplete()
	select {
	case <-time.After(quicConfig.HandshakeIdleTimeout):
		conn.CloseWithError(401, "Gone")
		return
	case <-ctx.Done():
		conn.CloseWithError(401, "Gone")
		return
	case <-hsCtx.Done():
	}

	cs := conn.ConnectionState().TLS

	cfg, ok := a.mux.Load(cs.NegotiatedProtocol)
	if !ok {
		conn.CloseWithError(404, "Unsupported protocol")
		return
	}
	cfg.acceptor.Handle(conn)
}
