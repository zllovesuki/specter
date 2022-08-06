package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/acceptor"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/gateway/httprate"

	"github.com/go-chi/chi/v5"
	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/http3"
	"go.uber.org/zap"
)

type GatewayConfig struct {
	Logger       *zap.Logger
	Tun          tun.Server
	H2Listener   net.Listener
	H3Listener   quic.EarlyListener
	StatsHandler http.HandlerFunc
	RootDomain   string
	GatewayPort  int
	AdminUser    string
	AdminPass    string
}

type Gateway struct {
	GatewayConfig
	apexServer *apexServer
	altHeaders string

	http2TunnelAcceptor *acceptor.HTTP2Acceptor
	http3TunnelAcceptor *acceptor.HTTP3Acceptor

	http2ApexAcceptor *acceptor.HTTP2Acceptor
	http3ApexAcceptor *acceptor.HTTP3Acceptor

	h2ApexServer   *http.Server
	h2TunnelServer *http.Server
	h3ApexServer   *http3.Server
	h3TunnelServer *http3.Server
}

func New(conf GatewayConfig) (*Gateway, error) {
	g := &Gateway{
		GatewayConfig:       conf,
		http2TunnelAcceptor: acceptor.NewH2Acceptor(conf.H2Listener),
		http2ApexAcceptor:   acceptor.NewH2Acceptor(conf.H2Listener),
		http3TunnelAcceptor: acceptor.NewH3Acceptor(conf.H3Listener),
		http3ApexAcceptor:   acceptor.NewH3Acceptor(conf.H3Listener),
		apexServer: &apexServer{
			statsHandler: conf.StatsHandler,
			limiter:      httprate.LimitAll(10, time.Second), // limit request to apex endpoint to 10 req/s
			rootDomain:   conf.RootDomain,
			authUser:     conf.AdminUser,
			authPass:     conf.AdminPass,
		},
	}
	qCfg := &quic.Config{
		HandshakeIdleTimeout: time.Second * 5,
		KeepAlivePeriod:      time.Second * 30,
		MaxIdleTimeout:       time.Second * 60,
	}

	g.h2ApexServer = &http.Server{
		ReadHeaderTimeout: time.Second * 5,
		Handler:           g.apexMux(false),
	}
	g.h2TunnelServer = &http.Server{
		ReadHeaderTimeout: time.Second * 5,
		Handler:           g.httpHandler(false),
	}
	g.h3ApexServer = &http3.Server{
		Port:            conf.GatewayPort,
		QuicConfig:      qCfg,
		EnableDatagrams: false,
		Handler:         g.apexMux(true),
	}
	g.h3TunnelServer = &http3.Server{
		Port:            conf.GatewayPort,
		QuicConfig:      qCfg,
		EnableDatagrams: false,
		Handler:         g.httpHandler(true),
	}
	g.altHeaders = generateAltHeaders(conf.GatewayPort)
	if conf.AdminUser == "" || conf.AdminPass == "" {
		conf.Logger.Info("Missing credentials for internal endpoint, disabling endpoint")
	}
	return g, nil
}

func (g *Gateway) apexMux(h3 bool) http.Handler {
	r := chi.NewRouter()
	r.Use(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			g.appendHeaders(h3)(w.Header())
			h.ServeHTTP(w, r)
		})
	})
	g.apexServer.Mount(r)
	return r
}

func (g *Gateway) Start(ctx context.Context) {
	g.Logger.Info("gateway server started")

	go g.h2TunnelServer.Serve(g.http2TunnelAcceptor)
	go g.h2ApexServer.Serve(g.http2ApexAcceptor)
	go g.acceptHTTP2(ctx)

	go g.h3TunnelServer.ServeListener(g.http3TunnelAcceptor)
	go g.h3ApexServer.ServeListener(g.http3ApexAcceptor)
	go g.acceptHTTP3(ctx)
}

func (g *Gateway) Close() {
	g.h3ApexServer.Close()
	g.h3TunnelServer.Close()
	g.http2ApexAcceptor.Close()
	g.http2TunnelAcceptor.Close()
}

func (g *Gateway) acceptHTTP2(ctx context.Context) {
	for {
		conn, err := g.H2Listener.Accept()
		if err != nil {
			return
		}
		tconn := conn.(*tls.Conn)
		go g.handleH2Connection(ctx, tconn)
	}
}

func (g *Gateway) acceptHTTP3(ctx context.Context) {
	for {
		conn, err := g.H3Listener.Accept(ctx)
		if err != nil {
			return
		}
		go g.handleH3Connection(ctx, conn)
	}
}

func (g *Gateway) handleH3Connection(ctx context.Context, conn quic.EarlyConnection) {
	cs := conn.ConnectionState().TLS
	logger := g.Logger.With(zap.Bool("via-quic", true), zap.String("proto", cs.NegotiatedProtocol))

	switch cs.ServerName {
	case g.RootDomain:
		logger.Debug("forwarding apex connection", zap.String("hostname", cs.ServerName))
		g.http3ApexAcceptor.Conn <- conn
	default:
		// maybe tunnel it
		switch cs.NegotiatedProtocol {
		case tun.ALPN(protocol.Link_TCP):
			for {
				stream, err := conn.AcceptStream(ctx)
				if err != nil {
					return
				}
				logger.Debug("forwarding tcp connection", zap.String("hostname", cs.ServerName))
				go g.handleH3Stream(ctx, cs.ServerName, stream)
			}
		default:
			logger.Debug("forwarding http connection", zap.String("hostname", cs.ServerName))
			g.http3TunnelAcceptor.Conn <- conn
		}
	}
}

func (g *Gateway) handleH2Connection(ctx context.Context, conn *tls.Conn) {
	var err error
	logger := g.Logger.With(zap.Bool("via-quic", false))

	defer func() {
		if err != nil {
			logger.Debug("handle connection failure", zap.Error(err))
			conn.Close()
		}
	}()

	hsCtx, hsCancel := context.WithTimeout(ctx, time.Second)
	defer hsCancel()

	err = conn.HandshakeContext(hsCtx)
	if err != nil {
		return
	}

	cs := conn.ConnectionState()
	logger = logger.With(zap.String("proto", cs.NegotiatedProtocol))

	switch cs.ServerName {
	case g.RootDomain:
		logger.Debug("forwarding apex connection", zap.String("hostname", cs.ServerName))
		g.http2ApexAcceptor.Conn <- conn
	default:
		// maybe tunnel it
		switch cs.NegotiatedProtocol {
		case tun.ALPN(protocol.Link_TCP):
			g.Logger.Debug("forwarding tcp connection", zap.String("hostname", cs.ServerName))
			err = g.forwardTCP(ctx, cs.ServerName, conn)
		case tun.ALPN(protocol.Link_UNKNOWN), tun.ALPN(protocol.Link_HTTP), tun.ALPN(protocol.Link_HTTP2):
			logger.Debug("forwarding http connection", zap.String("hostname", cs.ServerName))
			g.http2TunnelAcceptor.Conn <- conn
		default:
			err = fmt.Errorf("unknown alpn proposal")
		}
	}
}

func (g *Gateway) handleH3Stream(ctx context.Context, host string, stream quic.Stream) {
	var err error
	defer func() {
		if err != nil {
			g.Logger.Debug("handle connection failure", zap.Error(err), zap.Bool("via-quic", true))
			stream.Close()
		}
	}()
	err = g.forwardTCP(ctx, host, stream)
}

func (g *Gateway) forwardTCP(ctx context.Context, host string, conn io.ReadWriteCloser) error {
	var c net.Conn
	parts := strings.SplitN(host, ".", 2)
	c, err := g.Tun.Dial(ctx, &protocol.Link{
		Alpn:     protocol.Link_TCP,
		Hostname: parts[0],
	})
	if err != nil {
		return err
	}
	go tun.Pipe(conn, c)
	return nil
}
