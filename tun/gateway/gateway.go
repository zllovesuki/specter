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

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"

	"github.com/go-chi/chi/v5"
	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/http3"
	"go.uber.org/zap"
)

type GatewayConfig struct {
	Logger      *zap.Logger
	Tun         tun.Server
	H2Listener  net.Listener
	H3Listener  quic.EarlyListener
	RootDomain  string
	GatewayPort int
	ClientPort  int
}

type Gateway struct {
	GatewayConfig
	http2TunnelAcceptor *tun.HTTP2Acceptor
	http3TunnelAcceptor *tun.HTTP3Acceptor

	http2ApexAcceptor *tun.HTTP2Acceptor
	http3ApexAcceptor *tun.HTTP3Acceptor

	apexServer     *apexServer
	h3Enabled      bool
	h3ApexServer   *http3.Server
	h3TunnelServer *http3.Server
}

func New(conf GatewayConfig) (*Gateway, error) {
	g := &Gateway{
		GatewayConfig: conf,
		http2TunnelAcceptor: &tun.HTTP2Acceptor{
			Parent: conf.H2Listener,
			Conn:   make(chan net.Conn, 16),
		},
		http3TunnelAcceptor: &tun.HTTP3Acceptor{
			Parent: conf.H3Listener,
			Conn:   make(chan quic.EarlyConnection, 16),
		},
		http2ApexAcceptor: &tun.HTTP2Acceptor{
			Parent: conf.H2Listener,
			Conn:   make(chan net.Conn, 16),
		},
		http3ApexAcceptor: &tun.HTTP3Acceptor{
			Parent: conf.H3Listener,
			Conn:   make(chan quic.EarlyConnection, 16),
		},
		apexServer: &apexServer{
			rootDomain: conf.RootDomain,
			clientPort: conf.ClientPort,
		},
		h3ApexServer: &http3.Server{
			Port: conf.GatewayPort,
			QuicConfig: &quic.Config{
				HandshakeIdleTimeout: time.Second * 5,
				KeepAlivePeriod:      time.Second * 30,
				MaxIdleTimeout:       time.Second * 60,
			},
		},
		h3TunnelServer: &http3.Server{
			Port: conf.GatewayPort,
			QuicConfig: &quic.Config{
				HandshakeIdleTimeout: time.Second * 5,
				KeepAlivePeriod:      time.Second * 30,
				MaxIdleTimeout:       time.Second * 60,
			},
		},
		h3Enabled: conf.H3Listener != nil,
	}
	g.h3ApexServer.Handler = g.apexMux(true)
	g.h3TunnelServer.Handler = g.httpHandler(true)
	return g, nil
}

func (g *Gateway) apexMux(h3 bool) http.Handler {
	r := chi.NewRouter()
	if !h3 {
		r.Use(func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				g.h3ApexServer.SetQuicHeaders(w.Header())
				h.ServeHTTP(w, r)
			})
		})
	}
	g.apexServer.Mount(r)
	return r
}

func (g *Gateway) Start(ctx context.Context) {
	g.Logger.Info("gateway server started")
	go http.Serve(g.http2TunnelAcceptor, g.httpHandler(false))
	go http.Serve(g.http2ApexAcceptor, g.apexMux(false))
	go g.acceptHTTP2(ctx)
	if g.h3Enabled {
		g.Logger.Info("enabling http3 tunnel gateway support")
		go g.h3TunnelServer.ServeListener(g.http3TunnelAcceptor)
		go g.h3ApexServer.ServeListener(g.http3ApexAcceptor)
		go g.acceptHTTP3(ctx)
	}
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
	hsCtx := conn.HandshakeComplete()
	select {
	case <-time.After(time.Second):
		return
	case <-ctx.Done():
		return
	case <-hsCtx.Done():
	}

	logger := g.Logger.With(zap.Bool("via-quic", true))

	cs := conn.ConnectionState().TLS

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
			err = fmt.Errorf("unknown alpn proposal: %s", cs.NegotiatedProtocol)
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
