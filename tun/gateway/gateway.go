package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/gateway/httprate"
	"kon.nect.sh/specter/util/acceptor"

	"github.com/go-chi/chi/v5"
	"github.com/libp2p/go-yamux/v3"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"moul.io/zapfilter"
)

type DeadlineReadWriteCloser interface {
	io.ReadWriteCloser
	SetReadDeadline(time.Time) error
}

type GatewayConfig struct {
	Tun          tun.Server
	HTTPListener net.Listener
	H2Listener   net.Listener
	H3Listener   quic.EarlyListener
	Logger       *zap.Logger
	StatsHandler http.HandlerFunc
	RootDomain   string
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

func getStdLogger(parent *zap.Logger, sub string) *log.Logger {
	logger, err := zap.NewStdLogAt(parent.With(zap.String("subsystem", sub)), zapcore.WarnLevel)
	if err != nil {
		panic(fmt.Errorf("error getting proxy logger: %w", err))
	}
	return logger
}

func New(conf GatewayConfig) *Gateway {
	g := &Gateway{
		GatewayConfig:       conf,
		httpTunnelAcceptor:  acceptor.NewH2Acceptor(conf.H2Listener),
		tcpApexAcceptor:     acceptor.NewH2Acceptor(conf.H2Listener),
		http3TunnelAcceptor: acceptor.NewH3Acceptor(conf.H3Listener),
		quicApexAcceptor:    acceptor.NewH3Acceptor(conf.H3Listener),
		apexServer: &apexServer{
			statsHandler: conf.StatsHandler,
			limiter:      httprate.LimitAll(10, time.Second), // limit request to apex endpoint to 10 req/s
			rootDomain:   conf.RootDomain,
			authUser:     conf.AdminUser,
			authPass:     conf.AdminPass,
		},
	}

	// filter out unproductive messages
	filteredLogger := zap.New(zapfilter.NewFilteringCore(
		g.Logger.Core(),
		func(e zapcore.Entry, f []zapcore.Field) bool {
			return !strings.HasPrefix(e.Message, "http: URL query contains semicolon")
		}),
	)

	qCfg := &quic.Config{
		HandshakeIdleTimeout: time.Second * 5,
		KeepAlivePeriod:      time.Second * 30,
		MaxIdleTimeout:       time.Second * 60,
	}
	apex := g.apexMux()
	proxyHandler := g.proxyHandler(getStdLogger(filteredLogger, "httpProxy"))
	g.tcpApexServer = &http.Server{
		ReadHeaderTimeout: time.Second * 5,
		Handler:           apex,
		ErrorLog:          getStdLogger(filteredLogger, "tcpApex"),
	}
	g.h2TunnelServer = &http.Server{
		ReadHeaderTimeout: time.Second * 5,
		Handler:           proxyHandler,
		ErrorLog:          getStdLogger(filteredLogger, "h2Tunnel"),
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
		ErrorLog:          getStdLogger(filteredLogger, "localApex"),
	}
	g.httpServer = &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       5 * time.Second,
		Handler:           http.HandlerFunc(g.httpRedirect),
		ErrorLog:          getStdLogger(filteredLogger, "httpRedirect"),
	}
	g.altHeaders = generateAltHeaders(conf.GatewayPort)
	if conf.AdminUser == "" || conf.AdminPass == "" {
		conf.Logger.Info("Missing credentials for internal endpoint, disabling endpoint")
	}
	return g
}

func (g *Gateway) apexMux() http.Handler {
	r := chi.NewRouter()
	r.Use(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			g.appendHeaders(r.ProtoAtLeast(3, 0))(w.Header())
			h.ServeHTTP(w, r)
		})
	})
	g.apexServer.Mount(r)
	return r
}

func (g *Gateway) Start(ctx context.Context) {
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
		g.Logger.Info("Enabling HTTP (80) redirect to HTTPS (443)")
		go g.httpServer.Serve(g.HTTPListener)
	}

	g.Logger.Info("gateway server started")

	<-ctx.Done()
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
	logger := g.Logger.With(zap.Bool("via-quic", true),
		zap.String("proto", cs.NegotiatedProtocol),
		zap.String("tls.ServerName", cs.ServerName),
	)

	switch cs.ServerName {
	case "":
		q.CloseWithError(0, "")
	case g.RootDomain:
		logger.Debug("forwarding apex connection")
		g.quicApexAcceptor.Handle(q)
	default:
		// maybe tunnel it
		switch cs.NegotiatedProtocol {
		case tun.ALPN(protocol.Link_TCP):
			g.handleH3Multiplex(ctx, logger, q, cs.ServerName)
		default:
			logger.Debug("forwarding http connection")
			g.http3TunnelAcceptor.Handle(q)
		}
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
	logger := g.Logger.With(zap.Bool("via-quic", false),
		zap.String("proto", cs.NegotiatedProtocol),
		zap.String("tls.ServerName", cs.ServerName))

	switch cs.ServerName {
	case "":
		conn.Close()
	case g.RootDomain:
		logger.Debug("forwarding apex connection")
		g.tcpApexAcceptor.Handle(conn)
	default:
		// maybe tunnel it
		switch cs.NegotiatedProtocol {
		case tun.ALPN(protocol.Link_TCP):
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

func (g *Gateway) forwardTCP(ctx context.Context, host string, remote string, conn DeadlineReadWriteCloser) error {
	var c net.Conn
	var err error
	defer func() {
		if err != nil {
			tun.SendStatusProto(conn, err)
			conn.Close()
			return
		}
		go tun.Pipe(conn, c)
	}()

	// because of quic's early connection, the client need to "poke" us before
	// we can actually accept a stream, despite .OpenStreamSync
	conn.SetReadDeadline(time.Now().Add(time.Second * 3))
	err = tun.DrainStatusProto(conn)
	if err != nil {
		return err
	}
	conn.SetReadDeadline(time.Time{})

	parts := strings.SplitN(host, ".", 2)
	c, err = g.Tun.Dial(ctx, &protocol.Link{
		Alpn:     protocol.Link_TCP,
		Hostname: parts[0],
		Remote:   remote,
	})
	if err != nil {
		return err
	}
	return nil
}
