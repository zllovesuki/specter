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

	"specter/spec/protocol"
	"specter/spec/rpc"
	"specter/spec/transport"
	"specter/spec/tun"

	"go.uber.org/zap"
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
	rpc             rpc.RPC
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
	local, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:8080")
	if err != nil {
		c.logger.Error("forwarding connection", zap.Error(err))
		remote.Close()
		return
	}
	tun.Pipe(remote, local)
}

func (c *Client) registerTunnel() {
	req := &protocol.RPC_Request{
		Kind:            protocol.RPC_GET_NODES,
		GetNodesRequest: &protocol.GetNodesRequest{},
	}
	resp, err := c.rpc.Call(context.TODO(), req)
	if err != nil {
		panic(err)
	}
	for _, node := range resp.GetNodesResponse.GetNodes() {
		c.logger.Debug("get nodes", zap.String("node", node.String()))
	}
}

func (c *Client) Tunnel() {
	overwrite := false
	u, err := url.Parse("http://127.0.0.1:8080")
	if err != nil {
		c.logger.Fatal("parsing forwarding target", zap.Error(err))
	}
	switch u.Scheme {
	case "http", "https", "tcp":
	default:
		c.logger.Fatal("unsupported scheme. valid schemes: http, https, tcp", zap.String("scheme", u.Scheme))
	}

	hostname := "example.example.com"

	tunnelURL, _ := url.Parse(hostname)
	hostHeader := u.Host
	if overwrite {
		hostHeader = tunnelURL.Host
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

	connCh := make(chan net.Conn, 32)
	accepter := &tun.HTTPAcceptor{
		Conn: connCh,
	}
	forwarder := &http.Server{
		Handler:  proxy,
		ErrorLog: zap.NewStdLog(c.logger),
	}
	go func() {
		forwarder.Serve(accepter)
	}()

	for delegation := range c.serverTransport.Direct() {
		connCh <- delegation.Connection
	}
}
