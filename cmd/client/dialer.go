package client

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
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

// TODO: can we unit test this somehow
var rebootstrapRetry = time.Second * 5

type bootstrapFn func() (net.Addr, error)
type TransportDialer func() (net.Conn, error)

func TLSDialer(ctx *cli.Context, logger *zap.Logger, parsed *ParsedApex, norebootstrap bool) (net.Addr, TransportDialer, error) {
	clientTLSConf := &tls.Config{
		ServerName:         parsed.Host,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NextProtos: []string{
			tun.ALPN(protocol.Link_TCP),
		},
	}
	// used in integration test
	if v, ok := ctx.App.Metadata["connectOverride"]; ok {
		clientTLSConf.ServerName = v.(string)
	}

	dialer := &tls.Dialer{
		Config: clientTLSConf,
	}

	var (
		bootstrap bootstrapFn
		aSession  atomic.Value
	)

	bootstrap = func() (net.Addr, error) {
		logger.Debug("bootstrapping yamux")
		openCtx, cancel := context.WithTimeout(ctx.Context, transport.ConnectTimeout)
		defer cancel()

		conn, err := dialer.DialContext(openCtx, "tcp", parsed.String())
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
		if !norebootstrap {
			go rebootstrap(ctx.Context, logger, bootstrap, session.CloseChan())
		}

		return conn.RemoteAddr(), nil
	}

	remote, err := bootstrap()
	if err != nil {
		return nil, nil, err
	}

	return remote, func() (net.Conn, error) {
		session := aSession.Load().(*yamux.Session)
		r, err := session.OpenStream(ctx.Context)
		if err != nil {
			return nil, err
		}
		return r, nil
	}, nil
}

func QuicDialer(ctx *cli.Context, logger *zap.Logger, parsed *ParsedApex, norebootstrap bool) (net.Addr, TransportDialer, error) {
	clientTLSConf := &tls.Config{
		ServerName:         parsed.Host,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NextProtos: []string{
			tun.ALPN(protocol.Link_TCP),
		},
	}
	// used in integration test
	if v, ok := ctx.App.Metadata["connectOverride"]; ok {
		clientTLSConf.ServerName = v.(string)
	}

	var (
		bootstrap bootstrapFn
		aQuic     atomic.Value
	)

	bootstrap = func() (net.Addr, error) {
		logger.Debug("bootstrapping quic")
		q, err := quic.DialAddrContext(ctx.Context, parsed.String(), clientTLSConf, &quic.Config{
			KeepAlivePeriod:      time.Second * 5,
			HandshakeIdleTimeout: transport.ConnectTimeout,
			MaxIdleTimeout:       time.Second * 30,
			EnableDatagrams:      true,
		})
		if err != nil {
			return nil, err
		}

		aQuic.Store(q)
		if !norebootstrap {
			go rebootstrap(ctx.Context, logger, bootstrap, q.Context().Done())
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
