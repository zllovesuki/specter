package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/acceptor"
	"kon.nect.sh/specter/spec/protocol"
	rpcSpec "kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"

	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// Tunnel create procedure:
// client connects to server with random ID (TODO: use delegation so we can generate it)
// Transport exchanges identities and notify us as part of transport.Delegate
// at this point, we have a bidirectional Transport with client
// client then should request X more nodes from us, say 2 successors
// client then connects to X more nodes
// all those nodes should also have bidrectional Transport with client
// client then issue RPC to save 3 copies of (ClientIdentity, ServerIdentity) to DHT
// keys: chordHash(hostname-[1..3])
// tunnel is now registered

type Client struct {
	logger          *zap.Logger
	serverTransport transport.Transport
	rpc             rpcSpec.RPC
}

func NewClient(ctx context.Context, logger *zap.Logger, t transport.Transport, server *protocol.Node) (*Client, error) {
	r, err := t.DialRPC(ctx, server, nil)
	if err != nil {
		return nil, err
	}
	c := &Client{
		logger:          logger,
		serverTransport: t,
		rpc:             r,
	}

	return c, nil
}

func (c *Client) forward(ctx context.Context, remote net.Conn) {
	dialer := &net.Dialer{
		Timeout: time.Second * 3,
	}
	local, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:3000")
	if err != nil {
		c.logger.Error("forwarding connection", zap.Error(err))
		remote.Close()
		return
	}
	tun.Pipe(remote, local)
}

func (c *Client) GetCandidates(ctx context.Context) ([]*protocol.Node, error) {
	req := &protocol.RPC_Request{
		Kind:            protocol.RPC_GET_NODES,
		GetNodesRequest: &protocol.GetNodesRequest{},
	}
	resp, err := c.rpc.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetGetNodesResponse().GetNodes(), nil
}

func (c *Client) PublishTunnel(ctx context.Context, servers []*protocol.Node) (string, error) {
	req := &protocol.RPC_Request{
		Kind: protocol.RPC_PUBLISH_TUNNEL,
		PublishTunnelRequest: &protocol.PublishTunnelRequest{
			Client:  c.serverTransport.Identity(),
			Servers: servers,
		},
	}
	resp, err := c.rpc.Call(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.GetPublishTunnelResponse().GetHostname(), nil
}

func (c *Client) Tunnel(ctx context.Context, hostname string) {
	overwrite := false
	u, err := url.Parse("http://127.0.0.1:3000")
	if err != nil {
		c.logger.Fatal("parsing forwarding target", zap.Error(err))
	}
	switch u.Scheme {
	case "http", "https", "tcp":
	default:
		c.logger.Fatal("unsupported scheme. valid schemes: http, https, tcp", zap.String("scheme", u.Scheme))
	}

	tunnelURL, _ := url.Parse(hostname)
	hostHeader := u.Host
	if overwrite {
		hostHeader = tunnelURL.Hostname()
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	d := proxy.Director
	// https://stackoverflow.com/a/53007606
	// need to overwrite Host field
	proxy.Director = func(r *http.Request) {
		d(r)
		r.Host = hostHeader
	}
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, e error) {
		c.logger.Error("forwarding http/https request", zap.Error(e))
		rw.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(rw, "Forwarding target returned error: %s", e.Error())
	}
	proxy.ErrorLog = zap.NewStdLog(c.logger)

	httpCh := make(chan net.Conn, 32)
	accepter := &acceptor.HTTP2Acceptor{
		Conn: httpCh,
	}
	h2s := &http2.Server{}
	forwarder := &http.Server{
		Handler:  h2c.NewHandler(proxy, h2s),
		ErrorLog: zap.NewStdLog(c.logger),
	}
	go func() {
		forwarder.Serve(accepter)
	}()

	for delegation := range c.serverTransport.Direct() {
		go func(delegation *transport.StreamDelegate) {
			link := &protocol.Link{}
			if err := rpc.Receive(delegation.Connection, link); err != nil {
				c.logger.Error("receiving link information from gateway", zap.Error(err))
				delegation.Connection.Close()
				return
			}
			switch link.GetAlpn() {
			case protocol.Link_HTTP:
				httpCh <- delegation.Connection
			case protocol.Link_TCP:
				c.forward(ctx, delegation.Connection)
			default:
				c.logger.Error("unknown alpn for forwarding", zap.String("alpn", link.GetAlpn().String()))
				delegation.Connection.Close()
			}
		}(delegation)
	}
}
