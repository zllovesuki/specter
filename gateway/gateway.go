package gateway

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util"
	"kon.nect.sh/specter/util/acceptor"

	"github.com/go-chi/chi/v5"
	"github.com/libp2p/go-yamux/v4"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"kon.nect.sh/httprate"
	"moul.io/zapfilter"
)

type DeadlineReadWriteCloser interface {
	io.ReadWriteCloser
	SetReadDeadline(time.Time) error
}

type InternalHandlers struct {
	ChordStats       http.Handler
	ClientsQuery     http.Handler
	MigrationHandler http.HandlerFunc
}

type GatewayConfig struct {
	PKIServer    protocol.PKIService
	TunnelServer tun.Server
	HTTPListener net.Listener
	H2Listener   net.Listener
	H3Listener   quic.EarlyListener
	Logger       *zap.Logger
	Handlers     InternalHandlers
	RootDomains  []string
	AdminUser    string
	AdminPass    string
	GatewayPort  int
}

type Gateway struct {
	apexServer          *apexServer
	httpTunnelAcceptor  *acceptor.HTTP2Acceptor
	http3TunnelAcceptor *acceptor.HTTP3Acceptor
	tcpApexAcceptor     *acceptor.HTTP2Acceptor
	quicApexAcceptor    *acceptor.HTTP3Acceptor
	tcpApexServer       *http.Server
	h2TunnelServer      *http.Server
	localApexServer     *http.Server
	httpServer          *http.Server
	quicApexServer      *http3.Server
	h3TunnelServer      *http3.Server
	altHeaders          string
	GatewayConfig
}

func New(conf GatewayConfig) *Gateway {
	if conf.AdminUser == "" || conf.AdminPass == "" {
		conf.Logger.Info("Missing credentials for internal endpoint, disabling endpoint")
	}
	if conf.PKIServer != nil {
		conf.Logger.Info("Enabling client certificate issuance")
	}

	g := &Gateway{
		GatewayConfig:       conf,
		httpTunnelAcceptor:  acceptor.NewH2Acceptor(conf.H2Listener),
		tcpApexAcceptor:     acceptor.NewH2Acceptor(conf.H2Listener),
		http3TunnelAcceptor: acceptor.NewH3Acceptor(conf.H3Listener),
		quicApexAcceptor:    acceptor.NewH3Acceptor(conf.H3Listener),
	}

	// filter out unproductive messages
	filteredLogger := zap.New(zapfilter.NewFilteringCore(
		g.Logger.Core(),
		func(e zapcore.Entry, f []zapcore.Field) bool {
			if strings.HasPrefix(e.Message, "http: URL query contains semicolon") {
				return false
			}
			if strings.HasPrefix(e.Message, "suppressing panic for copyResponse error in test;") {
				return false
			}
			return true
		}),
	)
	proxyHandler := g.proxyHandler(util.GetStdLogger(filteredLogger, "httpProxy"))

	qCfg := &quic.Config{
		HandshakeIdleTimeout: time.Second * 5,
		KeepAlivePeriod:      time.Second * 30,
		MaxIdleTimeout:       time.Second * 60,
	}

	apex := chi.NewRouter()
	apex.Use(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			g.appendHeaders(r.ProtoAtLeast(3, 0))(w.Header())
			h.ServeHTTP(w, r)
		})
	})
	g.apexServer = &apexServer{
		handlers:      conf.Handlers,
		limiter:       httprate.LimitAll(10, time.Second), // limit request to apex endpoint to 10 req/s
		internalProxy: g.getInternalProxyHandler(),
		pkiServer:     conf.PKIServer,
		authUser:      conf.AdminUser,
		authPass:      conf.AdminPass,
	}
	g.apexServer.Mount(apex)
	g.tcpApexServer = &http.Server{
		ReadHeaderTimeout: time.Second * 5,
		Handler:           apex,
		ErrorLog:          util.GetStdLogger(filteredLogger, "tcpApex"),
	}
	g.h2TunnelServer = &http.Server{
		ReadHeaderTimeout: time.Second * 5,
		Handler:           proxyHandler,
		ErrorLog:          util.GetStdLogger(filteredLogger, "h2Tunnel"),
	}
	g.quicApexServer = &http3.Server{
		QuicConfig:      qCfg,
		EnableDatagrams: false,
		Handler:         apex,
	}
	g.h3TunnelServer = &http3.Server{
		QuicConfig:      qCfg,
		EnableDatagrams: false,
		Handler:         proxyHandler,
	}
	g.localApexServer = &http.Server{
		Addr:              "127.0.0.1:9999",
		ReadHeaderTimeout: time.Second * 5,
		Handler:           apex,
		ErrorLog:          util.GetStdLogger(filteredLogger, "localApex"),
	}
	g.httpServer = &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       5 * time.Second,
		Handler:           g.httpRouter(),
		ErrorLog:          util.GetStdLogger(filteredLogger, "httpServer"),
	}
	g.altHeaders = generateAltHeaders(conf.GatewayPort)

	return g
}

func (g *Gateway) AttachRouter(ctx context.Context, router *transport.StreamRouter) {
	router.HandleChord(protocol.Stream_INTERNAL, nil, func(delegate *transport.StreamDelegate) {
		g.tcpApexAcceptor.Handle(delegate)
	})
}

func (g *Gateway) MustStart(ctx context.Context) {
	// provide application context
	g.h2TunnelServer.BaseContext = func(l net.Listener) context.Context { return ctx }
	g.tcpApexServer.BaseContext = func(l net.Listener) context.Context { return ctx }
	g.localApexServer.BaseContext = func(l net.Listener) context.Context { return ctx }
	g.httpServer.BaseContext = func(l net.Listener) context.Context { return ctx }

	// start all servers
	go g.h2TunnelServer.Serve(g.httpTunnelAcceptor)
	go g.tcpApexServer.Serve(g.tcpApexAcceptor)

	go g.h3TunnelServer.ServeListener(g.http3TunnelAcceptor)
	go g.quicApexServer.ServeListener(g.quicApexAcceptor)

	go g.acceptTCP(ctx)
	go g.acceptQUIC(ctx)

	go g.localApexServer.ListenAndServe()

	if g.HTTPListener != nil {
		g.Logger.Info("Enabling HTTP Handler for HTTP Connect and HTTPS Redirect", zap.String("listen", g.HTTPListener.Addr().String()))
		go g.httpServer.Serve(g.HTTPListener)
	}

	g.Logger.Info("gateway server started")
}

func (g *Gateway) Close() {
	g.httpTunnelAcceptor.Close()

	g.tcpApexAcceptor.Close()
	g.quicApexAcceptor.Close()

	g.h2TunnelServer.Close()
	g.h3TunnelServer.Close()

	g.tcpApexServer.Close()
	g.quicApexServer.Close()

	g.localApexServer.Close()
}

func (g *Gateway) acceptTCP(ctx context.Context) {
	for {
		conn, err := g.H2Listener.Accept()
		if err != nil {
			return
		}
		tconn := conn.(*tls.Conn)
		go g.handleH2Connection(ctx, tconn)
	}
}

func (g *Gateway) acceptQUIC(ctx context.Context) {
	for {
		conn, err := g.H3Listener.Accept(ctx)
		if err != nil {
			return
		}
		go g.handleH3Connection(ctx, conn)
	}
}

func (g *Gateway) handleH3Connection(ctx context.Context, q quic.EarlyConnection) {
	cs := q.ConnectionState().TLS
	logger := g.Logger.With(
		zap.Bool("via-quic", true),
		zap.String("proto", cs.NegotiatedProtocol),
		zap.String("tls.ServerName", cs.ServerName),
	)

	if len(cs.ServerName) == 0 {
		q.CloseWithError(0, "")
		return
	}

	if util.Contains(g.RootDomains, cs.ServerName) {
		logger.Debug("forwarding apex connection")
		g.quicApexAcceptor.Handle(q)
		return
	}

	// maybe tunnel it
	switch cs.NegotiatedProtocol {
	case tun.ALPN(protocol.Link_TCP):
		logger.Debug("forwarding tcp connection")
		g.handleH3Multiplex(ctx, logger, q, cs.ServerName)
	default:
		logger.Debug("forwarding http connection")
		g.http3TunnelAcceptor.Handle(q)
	}
}

func (g *Gateway) handleH3Multiplex(ctx context.Context, logger *zap.Logger, q quic.EarlyConnection, host string) {
	for {
		stream, err := q.AcceptStream(ctx)
		if err != nil {
			return
		}
		go func(stream DeadlineReadWriteCloser) {
			if err := g.forwardTCP(ctx, host, q.RemoteAddr().String(), stream); err == nil {
				logger.Debug("forwarding tcp connection")
			}
		}(stream)
	}
}

func (g *Gateway) handleH2Connection(ctx context.Context, conn *tls.Conn) {
	hsCtx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()
	if err := conn.HandshakeContext(hsCtx); err != nil {
		conn.Close()
		return
	}

	cs := conn.ConnectionState()
	logger := g.Logger.With(
		zap.Bool("via-quic", false),
		zap.String("proto", cs.NegotiatedProtocol),
		zap.String("tls.ServerName", cs.ServerName),
	)

	if len(cs.ServerName) == 0 {
		conn.Close()
		return
	}

	if util.Contains(g.RootDomains, cs.ServerName) {
		logger.Debug("forwarding apex connection")
		g.tcpApexAcceptor.Handle(conn)
		return
	}

	// maybe tunnel it
	switch cs.NegotiatedProtocol {
	case tun.ALPN(protocol.Link_TCP):
		logger.Debug("forwarding tcp connection")
		cfg := yamux.DefaultConfig()
		cfg.LogOutput = io.Discard
		session, err := yamux.Server(conn, cfg, nil)
		if err != nil {
			return
		}
		g.handleH2Multiplex(ctx, logger, session, cs.ServerName)
	default:
		logger.Debug("forwarding http connection")
		g.httpTunnelAcceptor.Handle(conn)
	}
}

func (g *Gateway) handleH2Multiplex(ctx context.Context, logger *zap.Logger, session *yamux.Session, host string) {
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return
		}
		go func(stream DeadlineReadWriteCloser) {
			if err := g.forwardTCP(ctx, host, session.RemoteAddr().String(), stream); err == nil {
				logger.Debug("forwarding tcp connection")
			}
		}(stream)
	}
}
