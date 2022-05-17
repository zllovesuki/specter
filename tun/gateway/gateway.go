package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/zllovesuki/specter/spec/protocol"
	tunSpec "github.com/zllovesuki/specter/spec/tun"
	"github.com/zllovesuki/specter/tun/server"

	"go.uber.org/zap"
)

type GatewayConfig struct {
	Logger      *zap.Logger
	Tun         *server.Server
	Listener    net.Listener
	RootDomain  string
	GatewayPort int
}

type Gateway struct {
	GatewayConfig
	httpTunnelAcceptor *tunSpec.HTTPAcceptor
}

func New(conf GatewayConfig) (*Gateway, error) {
	if conf.GatewayPort != 443 {
		conf.RootDomain = fmt.Sprintf("%s:%d", conf.RootDomain, conf.GatewayPort)
	}
	return &Gateway{
		GatewayConfig: conf,
		httpTunnelAcceptor: &tunSpec.HTTPAcceptor{
			Parent: conf.Listener,
			Conn:   make(chan net.Conn, 16),
		},
	}, nil
}

func (g *Gateway) Start(ctx context.Context) {
	g.Logger.Info("gateway server started")
	go http.Serve(g.httpTunnelAcceptor, g.httpHandler())

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
	cs := conn.ConnectionState()
	switch cs.ServerName {
	case g.RootDomain:
		// root
		conn.Close()
	default:
		// maybe tunnel it
		switch cs.NegotiatedProtocol {
		case tunSpec.ALPN(protocol.Link_UNKNOWN), tunSpec.ALPN(protocol.Link_HTTP):
			g.Logger.Debug("forwarding http connection", zap.String("hostname", cs.ServerName))
			g.httpTunnelAcceptor.Conn <- conn

		case tunSpec.ALPN(protocol.Link_TCP):
			g.Logger.Debug("forwarding tcp connection", zap.String("hostname", cs.ServerName))
			parts := strings.SplitN(cs.ServerName, ".", 2)
			c, err := g.Tun.Dial(ctx, &protocol.Link{
				Alpn:     protocol.Link_TCP,
				Hostname: parts[0],
			})
			if err != nil {
				g.Logger.Error("establish raw link error", zap.Error(err))
				conn.Close()
			}
			go tunSpec.Pipe(conn, c)

		default:
			g.Logger.Warn("unknown alpn proposal", zap.String("proposal", cs.NegotiatedProtocol))
			conn.Close()
		}
	}
}
