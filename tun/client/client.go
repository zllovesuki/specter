package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/zhangyunhao116/skipmap"
	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/protocol"
	rpcSpec "kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util/acceptor"

	"go.uber.org/atomic"
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
	configMu        sync.RWMutex
	config          *Config
	rootDomain      *atomic.String
	proxies         *skipmap.StringMap[*httpProxy]
}

func NewClient(ctx context.Context, logger *zap.Logger, t transport.Transport, cfg *Config) (*Client, error) {
	server := &protocol.Node{
		Address: cfg.Apex,
	}

	r, err := t.DialRPC(ctx, server, nil)
	if err != nil {
		return nil, err
	}
	c := &Client{
		logger:          logger.With(zap.Uint64("id", t.Identity().GetId())),
		serverTransport: t,
		rpc:             r,
		config:          cfg,
		rootDomain:      atomic.NewString(""),
		proxies:         skipmap.NewString[*httpProxy](),
	}

	return c, nil
}

func (c *Client) getToken() *protocol.ClientToken {
	c.configMu.RLock()
	token := c.config.Token
	c.configMu.RUnlock()
	return &protocol.ClientToken{
		Token: []byte(token),
	}
}

func (c *Client) Register(ctx context.Context) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	if c.config.Token != "" {
		c.logger.Info("Found existing client token")
		req := &protocol.RPC_Request{
			Kind: protocol.RPC_CLIENT_REQUEST,
			ClientRequest: &protocol.ClientRequest{
				Kind: protocol.TunnelRPC_PING,
			},
		}
		resp, err := c.rpc.Call(ctx, req)
		if err != nil {
			return err
		}
		root := resp.GetClientResponse().GetRegisterResponse().GetApex()
		c.rootDomain.Store(root)

		return nil
	}

	req := &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind: protocol.TunnelRPC_IDENTITY,
			RegisterRequest: &protocol.RegisterIdentityRequest{
				Client: c.serverTransport.Identity(),
			},
		},
	}
	resp, err := c.rpc.Call(ctx, req)
	if err != nil {
		return err
	}
	token := resp.GetClientResponse().GetRegisterResponse().GetToken().GetToken()
	root := resp.GetClientResponse().GetRegisterResponse().GetApex()
	c.rootDomain.Store(root)
	c.config.Token = string(token)

	c.logger.Info("Client registered via apex")

	if err := c.config.writeFile(); err != nil {
		c.logger.Error("Error saving token to config file", zap.Error(err))
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	req := &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind:  protocol.TunnelRPC_PING,
			Token: c.getToken(),
		},
	}
	_, err := c.rpc.Call(ctx, req)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) RequestHostname(ctx context.Context) (string, error) {
	req := &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind:  protocol.TunnelRPC_HOSTNAME,
			Token: c.getToken(),
			RegisterRequest: &protocol.RegisterIdentityRequest{
				Client: c.serverTransport.Identity(),
			},
		},
	}
	resp, err := c.rpc.Call(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.GetClientResponse().GetHostnameResponse().GetHostname(), nil
}

func (c *Client) RequestCandidates(ctx context.Context) ([]*protocol.Node, error) {
	req := &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind:         protocol.TunnelRPC_NODES,
			Token:        c.getToken(),
			NodesRequest: &protocol.GetNodesRequest{},
		},
	}
	resp, err := c.rpc.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetClientResponse().GetNodesResponse().GetNodes(), nil
}

func (c *Client) SyncConfigTunnels(ctx context.Context, connected []*protocol.Node) {
	c.configMu.RLock()
	tunnels := append([]Tunnel{}, c.config.Tunnels...)
	c.configMu.RUnlock()

	c.logger.Info("Synchronizing tunnels in config file with specter", zap.Int("tunnels", len(tunnels)))

	for i, tunnel := range tunnels {
		if tunnel.Hostname == "" && tunnel.Target != "" {
			name, err := c.RequestHostname(ctx)
			if err != nil {
				c.logger.Error("Failed to request hostname", zap.String("target", tunnel.Target), zap.Error(err))
				continue
			}
			tunnels[i].Hostname = name
		}
	}

	for _, tunnel := range tunnels {
		if tunnel.Hostname == "" {
			continue
		}
		if err := c.PublishTunnel(ctx, tunnel.Hostname, connected); err != nil {
			c.logger.Error("Failed to publish tunnel", zap.String("hostname", tunnel.Hostname), zap.String("target", tunnel.Target), zap.Error(err))
			continue
		}
		c.logger.Info("Tunnel published", zap.String("hostname", fmt.Sprintf("%s.%s", tunnel.Hostname, c.rootDomain.Load())), zap.String("target", tunnel.Target))
	}

	c.configMu.Lock()
	c.config.Tunnels = tunnels
	if err := c.config.writeFile(); err != nil {
		c.logger.Error("Error saving token to config file", zap.Error(err))
	}
	c.config.buildRouter()
	c.configMu.Unlock()
}

func (c *Client) PublishTunnel(ctx context.Context, hostname string, connected []*protocol.Node) error {
	req := &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind:  protocol.TunnelRPC_TUNNEL,
			Token: c.getToken(),
			TunnelRequest: &protocol.PublishTunnelRequest{
				Hostname: hostname,
				Servers:  connected,
			},
		},
	}
	_, err := c.rpc.Call(ctx, req)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) Accept(ctx context.Context) {
	c.logger.Info("Listening for tunnel traffic")

	for delegation := range c.serverTransport.Direct() {
		go func(delegation *transport.StreamDelegate) {
			link := &protocol.Link{}
			if err := rpc.Receive(delegation.Connection, link); err != nil {
				c.logger.Error("Receiving link information from gateway", zap.Error(err))
				delegation.Connection.Close()
				return
			}
			hostname := link.Hostname
			u, ok := c.config.router.Load(hostname)
			if !ok {
				c.logger.Error("Unknown hostname in connection", zap.String("hostname", hostname))
				delegation.Connection.Close()
				return
			}

			c.logger.Info("incoming connection", zap.Any("link", link))

			switch link.GetAlpn() {
			case protocol.Link_HTTP:
				c.getProxy(ctx, hostname, u).acceptor.Handle(delegation.Connection)

			case protocol.Link_TCP:
				c.forward(ctx, delegation.Connection, u.Host)

			default:
				c.logger.Error("Unknown alpn for forwarding", zap.String("alpn", link.GetAlpn().String()))
				delegation.Connection.Close()
			}
		}(delegation)
	}
}

func (c *Client) forward(ctx context.Context, remote net.Conn, dest string) {
	dialer := &net.Dialer{
		Timeout: time.Second * 3,
	}
	local, err := dialer.DialContext(ctx, "tcp", dest)
	if err != nil {
		c.logger.Error("forwarding connection", zap.Error(err))
		remote.Close()
		return
	}
	tun.Pipe(remote, local)
}

func (c *Client) getProxy(ctx context.Context, hostname string, u *url.URL) *httpProxy {
	proxy, loaded := c.proxies.LoadOrStoreLazy(hostname, func() *httpProxy {
		c.logger.Info("Creating new proxy", zap.String("hostname", hostname))

		proxy := httputil.NewSingleHostReverseProxy(u)
		d := proxy.Director
		// https://stackoverflow.com/a/53007606
		// need to overwrite Host field
		proxy.Director = func(r *http.Request) {
			d(r)
			r.Host = u.Host
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

		accepter := acceptor.NewH2Acceptor(nil)
		h2s := &http2.Server{}
		forwarder := &http.Server{
			Handler:           h2c.NewHandler(proxy, h2s),
			ErrorLog:          zap.NewStdLog(c.logger),
			ReadHeaderTimeout: time.Second * 15,
		}

		return &httpProxy{
			acceptor:  accepter,
			forwarder: forwarder,
		}
	})
	if !loaded {
		go proxy.forwarder.Serve(proxy.acceptor)
	}
	return proxy
}

type httpProxy struct {
	acceptor  *acceptor.HTTP2Acceptor
	forwarder *http.Server
}
