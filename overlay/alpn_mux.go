package overlay

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"kon.nect.sh/specter/util/acceptor"

	"github.com/lucas-clemente/quic-go"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap"
)

type ALPNMux struct {
	logger   *zap.Logger
	listener quic.EarlyListener
	mux      *skipmap.StringMap[*protoCfg]
}

type protoCfg struct {
	acceptor *acceptor.HTTP3Acceptor
	tls      *tls.Config
}

func NewMux(logger *zap.Logger, addr string) (*ALPNMux, error) {
	a := &ALPNMux{
		logger: logger,
		mux:    skipmap.NewString[*protoCfg](),
	}
	listener, err := quic.ListenAddrEarly(addr, &tls.Config{
		GetConfigForClient: a.getConfigForClient,
	}, quicConfig)
	if err != nil {
		return nil, err
	}
	a.listener = listener
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
	protos := make([]string, 0)
	a.mux.Range(func(proto string, _ *protoCfg) bool {
		protos = append(protos, proto)
		return true
	})
	a.logger.Info("alpn muxer started", zap.Strings("protos", protos))
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
