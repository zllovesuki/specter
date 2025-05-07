package client

import (
	"context"
	"crypto/tls"
	"net"

	"go.miragespace.co/specter/gateway"
	"go.miragespace.co/specter/spec/cipher"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"

	"github.com/iangudger/memnet"
	"go.uber.org/zap"
)

type emulatedTunnelServer struct {
	cli *Client
}

func (e *emulatedTunnelServer) DialClient(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	// the pipe net.Conn must be buffered to preserved real-world network behavior
	p1, p2 := memnet.NewBufferedStreamConnPair()
	err := e.cli.handleIncomingDelegation(ctx, link, p1)
	if err != nil {
		return nil, err
	}
	return p2, nil
}

func (e *emulatedTunnelServer) DialInternal(context.Context, *protocol.Node) (net.Conn, error) {
	return nil, tun.ErrLookupFailed
}

func (e *emulatedTunnelServer) Identity() *protocol.Node {
	return &protocol.Node{
		Id:      e.cli.ServerTransport.Identity().GetId(),
		Address: e.cli.KeylessTCPListener.Addr().String(),
	}
}

var _ tun.Server = (*emulatedTunnelServer)(nil)

func (c *Client) startKeylessProxy() {
	if c.KeylessTCPListener == nil || c.KeylessALPNMux == nil {
		return
	}

	address := c.KeylessTCPListener.Addr()
	tcpAddress, ok := address.(*net.TCPAddr)
	if !ok {
		return
	}

	gwTLSConf := cipher.GetGatewayTLSConfig(c.getCertificate, []string{
		tun.ALPN(protocol.Link_HTTP2),
		tun.ALPN(protocol.Link_HTTP),
		tun.ALPN(protocol.Link_TCP),
		tun.ALPN(protocol.Link_UNKNOWN),
	})

	gwH2Listener := tls.NewListener(c.KeylessTCPListener, gwTLSConf)
	defer gwH2Listener.Close()

	// handles h3, h3-29, and specter-tcp/1
	gwH3Listener := c.KeylessALPNMux.With(gwTLSConf, append([]string{tun.ALPN(protocol.Link_TCP)}, cipher.H3Protos...)...)
	defer gwH3Listener.Close()

	gwCfg := gateway.GatewayConfig{
		TunnelServer: &emulatedTunnelServer{c},
		Logger:       c.Logger.With(zap.String("component", "keyless_proxy")),
		H2Listener:   gwH2Listener,
		H3Listener:   gwH3Listener,
		GatewayPort:  tcpAddress.Port,
		Options: gateway.Options{
			TransportBufferSize: 1 << 16,
			ProxyBufferSize:     1 << 16,
		},
	}

	gw := gateway.New(gwCfg)
	defer gw.Close()

	gw.MustStart(c.parentCtx)

	<-c.closeCh
}
