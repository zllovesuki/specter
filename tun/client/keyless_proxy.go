package client

import (
	"context"
	"crypto/tls"
	"net"

	"go.miragespace.co/specter/gateway"
	"go.miragespace.co/specter/spec/cipher"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util/bufconn"

	"go.uber.org/zap"
)

type emulatedTunnelServer struct {
	cli *Client
}

func (e *emulatedTunnelServer) DialClient(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	// the pipe net.Conn must be buffered to preserved real-world network behavior
	c1, c2 := bufconn.BufferedPipe(8192)
	err := e.cli.handleIncomingDelegation(ctx, link, c1)
	if err != nil {
		return nil, err
	}
	return c2, nil
}

func (e *emulatedTunnelServer) DialInternal(context.Context, *protocol.Node) (net.Conn, error) {
	return nil, tun.ErrLookupFailed
}

func (e *emulatedTunnelServer) Identity() *protocol.Node {
	return &protocol.Node{
		Id:      e.cli.ServerTransport.Identity().GetId(),
		Address: e.cli.KeylessProxy.HTTPSListner.Addr().String(),
	}
}

var _ tun.Server = (*emulatedTunnelServer)(nil)

func (c *Client) startKeylessProxy() {
	if c.KeylessProxy.ALPNMux == nil || c.KeylessProxy.HTTPSListner == nil {
		return
	}

	address := c.KeylessProxy.HTTPSListner.Addr()
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

	gwH2Listener := tls.NewListener(c.KeylessProxy.HTTPSListner, gwTLSConf)
	defer gwH2Listener.Close()

	// handles h3, h3-29, and specter-tcp/1
	gwH3Listener := c.KeylessProxy.ALPNMux.With(gwTLSConf, append([]string{tun.ALPN(protocol.Link_TCP)}, cipher.H3Protos...)...)
	defer gwH3Listener.Close()

	gwCfg := gateway.GatewayConfig{
		TunnelServer: &emulatedTunnelServer{c},
		Logger:       c.Logger.With(zap.String("component", "keyless_proxy")),
		HTTPListener: c.KeylessProxy.HTTPListner,
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
