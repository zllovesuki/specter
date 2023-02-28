package dialer

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync/atomic"
	"time"

	"kon.nect.sh/specter/overlay"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"

	"github.com/libp2p/go-yamux/v3"
	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

type DialerConfig struct {
	Logger             *zap.Logger
	Parsed             *ParsedApex
	InsecureSkipVerify bool
	NoReconnection     bool
}

type TransportDialer func() (net.Conn, error)

// TODO: can we unit test this somehow
var rebootstrapRetry = time.Second * 5

type bootstrapFn func() (net.Addr, error)

func TLSDialer(ctx context.Context, dCfg DialerConfig) (net.Addr, TransportDialer, error) {
	clientTLSConf := &tls.Config{
		ServerName:         dCfg.Parsed.Host,
		InsecureSkipVerify: dCfg.InsecureSkipVerify,
		NextProtos: []string{
			tun.ALPN(protocol.Link_TCP),
		},
	}

	dialer := &tls.Dialer{
		Config: clientTLSConf,
	}

	var (
		bootstrap bootstrapFn
		aSession  atomic.Value
	)

	bootstrap = func() (net.Addr, error) {
		dCfg.Logger.Debug("bootstrapping yamux")
		openCtx, cancel := context.WithTimeout(ctx, transport.ConnectTimeout)
		defer cancel()

		conn, err := dialer.DialContext(openCtx, "tcp", dCfg.Parsed.String())
		if err != nil {
			return nil, err
		}

		cfg := yamux.DefaultConfig()
		cfg.LogOutput = io.Discard
		session, err := yamux.Client(conn, cfg, nil)
		if err != nil {
			return nil, err
		}

		aSession.Store(session)
		if !dCfg.NoReconnection {
			go rebootstrap(ctx, dCfg.Logger, bootstrap, session.CloseChan())
		}
		go func() {
			select {
			case <-ctx.Done():
				session.Close()
			case <-session.CloseChan():
				return
			}
		}()

		return conn.RemoteAddr(), nil
	}

	remote, err := bootstrap()
	if err != nil {
		return nil, nil, err
	}

	return remote, func() (net.Conn, error) {
		session := aSession.Load().(*yamux.Session)
		r, err := session.OpenStream(ctx)
		if err != nil {
			return nil, err
		}
		return r, nil
	}, nil
}

func QuicDialer(ctx context.Context, dCfg DialerConfig) (net.Addr, TransportDialer, error) {
	clientTLSConf := &tls.Config{
		ServerName:         dCfg.Parsed.Host,
		InsecureSkipVerify: dCfg.InsecureSkipVerify,
		NextProtos: []string{
			tun.ALPN(protocol.Link_TCP),
		},
	}

	var (
		bootstrap bootstrapFn
		aQuic     atomic.Value
	)

	bootstrap = func() (net.Addr, error) {
		dCfg.Logger.Debug("bootstrapping quic")
		q, err := quic.DialAddrContext(ctx, dCfg.Parsed.String(), clientTLSConf, &quic.Config{
			KeepAlivePeriod:      time.Second * 5,
			HandshakeIdleTimeout: transport.ConnectTimeout,
			MaxIdleTimeout:       time.Second * 30,
			EnableDatagrams:      true,
		})
		if err != nil {
			return nil, err
		}

		aQuic.Store(q)
		if !dCfg.NoReconnection {
			go rebootstrap(ctx, dCfg.Logger, bootstrap, q.Context().Done())
		}

		return q.RemoteAddr(), nil
	}

	remote, err := bootstrap()
	if err != nil {
		return nil, nil, err
	}

	return remote, func() (net.Conn, error) {
		q := aQuic.Load().(quic.Connection)
		r, err := q.OpenStream()
		if err != nil {
			return nil, err
		}
		return overlay.WrapQuicConnection(r, q), nil
	}, nil
}

func rebootstrap(ctx context.Context, logger *zap.Logger, fn bootstrapFn, exit <-chan struct{}) {
	logger.Debug("re-bootstrap started")
	select {
	case <-ctx.Done():
		return
	case <-exit:
		logger.Info("Disconnected from gateway, re-bootstrapping")
	AGAIN:
		logger.Debug("calling bootstrapFn")
		remote, err := fn()
		if err != nil {
			logger.Error("Failed to re-bootstrap, retrying", zap.Error(err))
			time.Sleep(rebootstrapRetry)
			goto AGAIN
		}
		logger.Info("Connection to gateway re-established", zap.String("via", remote.String()))
	}
}
