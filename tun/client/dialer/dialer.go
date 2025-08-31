package dialer

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync/atomic"
	"time"

	"go.miragespace.co/specter/overlay"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/libp2p/go-yamux/v4"
	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

type DialerConfig struct {
	Logger             *zap.Logger
	Parsed             *ParsedApex
	InsecureSkipVerify bool
	NoReconnection     bool
}

type TransportDialer interface {
	Dial() (net.Conn, error)
	Remote() net.Addr
}

// TODO: can we unit test this somehow
var rebootstrapRetry = time.Second * 5

type bootstrapFn func() (net.Addr, error)

func TLSDialer(ctx context.Context, dCfg DialerConfig) (net.Addr, TransportDialer, error) {
	var (
		bootstrap bootstrapFn
		aSession  atomic.Value
	)

	clientTLSConf := &tls.Config{
		ServerName:         dCfg.Parsed.Host,
		InsecureSkipVerify: dCfg.InsecureSkipVerify,
		NextProtos: []string{
			tun.ALPN(protocol.Link_TCP),
		},
	}

	override := GetServerNameOverride(ctx)
	if override != "" {
		clientTLSConf.ServerName = override
	}

	dialer := &tls.Dialer{
		Config: clientTLSConf,
	}

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

	return remote, &tlsDialer{aSession, ctx}, nil
}

type tlsDialer struct {
	aSession atomic.Value
	ctx      context.Context
}

var _ TransportDialer = (*tlsDialer)(nil)

func (t *tlsDialer) Dial() (net.Conn, error) {
	session := t.aSession.Load().(*yamux.Session)
	return session.OpenStream(t.ctx)
}

func (t *tlsDialer) Remote() net.Addr {
	session := t.aSession.Load().(*yamux.Session)
	return session.RemoteAddr()
}

func QuicDialer(ctx context.Context, dCfg DialerConfig) (net.Addr, TransportDialer, error) {
	var (
		bootstrap bootstrapFn
		aQuic     atomic.Value
	)

	clientTLSConf := &tls.Config{
		ServerName:         dCfg.Parsed.Host,
		InsecureSkipVerify: dCfg.InsecureSkipVerify,
		NextProtos: []string{
			tun.ALPN(protocol.Link_TCP),
		},
	}

	override := GetServerNameOverride(ctx)
	if override != "" {
		clientTLSConf.ServerName = override
	}

	bootstrap = func() (net.Addr, error) {
		dCfg.Logger.Debug("bootstrapping quic")
		q, err := quic.DialAddrEarly(ctx, dCfg.Parsed.String(), clientTLSConf, &quic.Config{
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

	return remote, &quicDialer{aQuic}, nil
}

type quicDialer struct {
	aQuic atomic.Value // quic.Connection
}

var _ TransportDialer = (*quicDialer)(nil)

func (q *quicDialer) Dial() (net.Conn, error) {
	qConn := q.aQuic.Load().(*quic.Conn)
	r, err := qConn.OpenStream()
	if err != nil {
		return nil, err
	}
	return overlay.WrapQuicConnection(r, qConn), nil
}

func (q *quicDialer) Remote() net.Addr {
	qConn := q.aQuic.Load().(*quic.Conn)
	return qConn.RemoteAddr()
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
