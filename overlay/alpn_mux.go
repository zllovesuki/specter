package overlay

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"kon.nect.sh/specter/spec/acceptor"
	"kon.nect.sh/specter/spec/cipher"

	"github.com/lucas-clemente/quic-go"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap"
)

type ALPNMux struct {
	logger   *zap.Logger
	baseCfg  *tls.Config
	listener quic.EarlyListener
	mux      *skipmap.StringMap[*acceptor.HTTP3Acceptor]
}

func NewMux(logger *zap.Logger, addr string, provider cipher.CertProviderFunc) (*ALPNMux, error) {
	a := &ALPNMux{
		logger:  logger,
		mux:     skipmap.NewString[*acceptor.HTTP3Acceptor](),
		baseCfg: cipher.GetGatewayTLSConfig(provider, nil),
	}
	a.baseCfg.GetConfigForClient = a.getConfigForClient

	listener, err := quic.ListenAddrEarly(addr, a.baseCfg, quicConfig)
	if err != nil {
		return nil, err
	}
	a.listener = listener
	return a, nil
}

func (a *ALPNMux) getConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	var xCfg *tls.Config
	var found bool

	a.mux.Range(func(proto string, _ *acceptor.HTTP3Acceptor) bool {
		for _, cP := range hello.SupportedProtos {
			if proto == cP {
				found = true
				return false
			}
		}
		return true
	})
	if found {
		xCfg = a.baseCfg.Clone()
		xCfg.NextProtos = hello.SupportedProtos
		return xCfg, nil
	}
	return nil, errors.New("cipher: no mutually supported protocols")
}

func (a *ALPNMux) For(protos ...string) quic.EarlyListener {
	l := acceptor.NewH3Acceptor(a.listener)
	for _, proto := range protos {
		a.mux.Store(proto, l)
	}
	return l
}

func (a *ALPNMux) Aceept(ctx context.Context) {
	protos := make([]string, 0)
	a.mux.Range(func(proto string, _ *acceptor.HTTP3Acceptor) bool {
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
	a.mux.Range(func(_ string, acceptor *acceptor.HTTP3Acceptor) bool {
		acceptor.Close()
		return true
	})
	a.listener.Close()
}

func (a *ALPNMux) handleConnection(ctx context.Context, conn quic.EarlyConnection) {
	hsCtx := conn.HandshakeComplete()
	select {
	case <-time.After(quicConfig.HandshakeIdleTimeout):
		return
	case <-ctx.Done():
		return
	case <-hsCtx.Done():
	}

	cs := conn.ConnectionState().TLS

	acceptor, ok := a.mux.Load(cs.NegotiatedProtocol)
	if !ok {
		conn.CloseWithError(404, "Unsupported protocol")
		return
	}
	acceptor.Conn <- conn
}
