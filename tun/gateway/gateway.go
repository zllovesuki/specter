package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"

	"go.uber.org/zap"
)

type GatewayConfig struct {
	Logger      *zap.Logger
	Tun         tun.Server
	Listener    net.Listener
	RootDomain  string
	GatewayPort int
	ClientPort  int
}

type Gateway struct {
	GatewayConfig
	httpTunnelAcceptor *tun.HTTPAcceptor

	apexAcceptor *tun.HTTPAcceptor
	apexServer   *apexServer
}

func New(conf GatewayConfig) (*Gateway, error) {
	return &Gateway{
		GatewayConfig: conf,
		httpTunnelAcceptor: &tun.HTTPAcceptor{
			Parent: conf.Listener,
			Conn:   make(chan net.Conn, 16),
		},
		apexAcceptor: &tun.HTTPAcceptor{
			Parent: conf.Listener,
			Conn:   make(chan net.Conn, 16),
		},
		apexServer: &apexServer{
			rootDomain: conf.RootDomain,
			clientPort: conf.ClientPort,
		},
	}, nil
}

func (g *Gateway) Start(ctx context.Context) {
	g.Logger.Info("gateway server started")
	go http.Serve(g.httpTunnelAcceptor, g.httpHandler())
	go http.Serve(g.apexAcceptor, g.apexServer.Handler())

	for {
		conn, err := g.Listener.Accept()
		if err != nil {
			// g.Logger.Error("accepting gateway connection", zap.Error(err))
			return
		}
		tconn := conn.(*tls.Conn)
		go g.handleConnection(ctx, tconn)
	}
}

func (g *Gateway) handleConnection(ctx context.Context, conn *tls.Conn) {
	var err error

	defer func() {
		if err != nil {
			g.Logger.Debug("handle connection failure", zap.Error(err))
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
		g.apexAcceptor.Conn <- conn
		return
	default:
		// maybe tunnel it
		switch cs.NegotiatedProtocol {
		case tun.ALPN(protocol.Link_UNKNOWN), tun.ALPN(protocol.Link_HTTP), tun.ALPN(protocol.Link_HTTP2):
			g.httpTunnelAcceptor.Conn <- conn

		case tun.ALPN(protocol.Link_TCP):
			g.Logger.Debug("forwarding tcp connection", zap.String("hostname", cs.ServerName))
			var c net.Conn
			parts := strings.SplitN(cs.ServerName, ".", 2)
			c, err = g.Tun.Dial(ctx, &protocol.Link{
				Alpn:     protocol.Link_TCP,
				Hostname: parts[0],
			})
			if err != nil {
				return
			}
			go tun.Pipe(conn, c)

		default:
			err = fmt.Errorf("unknown alpn proposal: %s", cs.NegotiatedProtocol)
		}
	}
}
