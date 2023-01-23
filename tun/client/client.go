package client

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util/acceptor"

	"github.com/orisano/wyhash"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

const (
	checkInterval  = time.Second * 30
	connectTimeout = time.Second * 3
	rpcTimeout     = time.Second * 5
)

type Client struct {
	logger          *zap.Logger
	serverTransport transport.Transport
	configMu        sync.RWMutex
	syncMu          sync.Mutex
	config          *Config
	rootDomain      *atomic.String
	proxies         *skipmap.StringMap[*httpProxy]
	connections     *skipmap.Uint64Map[*connection]
}

type connection struct {
	rpc  rpc.RPC
	peer *protocol.Node
}

func NewClient(ctx context.Context, logger *zap.Logger, t transport.Transport, cfg *Config) (*Client, error) {
	c := &Client{
		logger:          logger.With(zap.Uint64("id", t.Identity().GetId())),
		serverTransport: t,
		config:          cfg,
		rootDomain:      atomic.NewString(""),
		proxies:         skipmap.NewString[*httpProxy](),
		connections:     skipmap.NewUint64[*connection](),
	}

	if err := c.openRPC(ctx, &protocol.Node{
		Address: cfg.Apex,
	}); err != nil {
		return nil, fmt.Errorf("error connecting to apex: %w", err)
	}

	return c, nil
}

func (c *Client) Initialize(ctx context.Context) error {
	if err := c.maintainConnections(ctx); err != nil {
		return err
	}
	c.SyncConfigTunnels(ctx)
	return nil
}

func (c *Client) openRPC(ctx context.Context, node *protocol.Node) error {
	if _, ok := c.connections.Load(node.GetId()); ok {
		return nil
	}

	r, err := c.serverTransport.DialRPC(ctx, node, nil)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	resp, err := c.Ping(callCtx, r)
	if err != nil {
		return err
	}

	identity := resp.GetNode()
	c.logger.Info("Connected to specter server", zap.String("addr", identity.GetAddress()))
	c.connections.Store(identity.GetId(), &connection{
		rpc:  r,
		peer: identity,
	})
	return nil
}

func (c *Client) rpcCall(ctx context.Context, req *protocol.ClientRequest) (*protocol.ClientResponse, error) {
	var candidate rpc.RPC
	c.connections.Range(func(id uint64, conn *connection) bool {
		candidate = conn.rpc
		return false
	})
	if candidate == nil {
		return nil, fmt.Errorf("no rpc candidates available")
	}
	resp, err := candidate.Call(ctx, &protocol.RPC_Request{
		Kind:          protocol.RPC_CLIENT_REQUEST,
		ClientRequest: req,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetClientResponse(), nil
}

func (c *Client) getConnectedNodes(ctx context.Context) []*protocol.Node {
	nodes := make([]*protocol.Node, 0)
	c.connections.Range(func(id uint64, conn *connection) bool {
		if len(nodes) < tun.NumRedundantLinks {
			nodes = append(nodes, conn.peer)
		}
		return true
	})
	return nodes
}

func (c *Client) getAliveNodes(ctx context.Context) (alive []*protocol.Node, dead int) {
	alive = make([]*protocol.Node, 0)
	c.connections.Range(func(id uint64, conn *connection) bool {
		func() {
			callCtx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()

			_, err := c.Ping(callCtx, conn.rpc)
			if err != nil {
				conn.rpc.Close()
				c.connections.Delete(id)
				dead++
			} else {
				alive = append(alive, conn.peer)
			}
		}()
		return true
	})
	return
}

func (c *Client) getToken() *protocol.ClientToken {
	c.configMu.RLock()
	token := c.config.Token
	c.configMu.RUnlock()
	return &protocol.ClientToken{
		Token: []byte(token),
	}
}

func (c *Client) hash(seed uint64, nodes []*protocol.Node) uint64 {
	var buf [8]byte

	hasher := wyhash.New(seed)

	for _, node := range nodes {
		binary.BigEndian.PutUint64(buf[:], node.GetId())
		hasher.Write(buf[:])
	}
	return hasher.Sum64()
}

func (c *Client) periodicReconnection(ctx context.Context) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prev, failed := c.getAliveNodes(ctx)
			if failed > 0 {
				c.logger.Info("Some connections have failed, opening more connections to specter server", zap.Int("dead", failed))
			}
			if err := c.maintainConnections(ctx); err != nil {
				continue
			}
			now, _ := c.getAliveNodes(ctx)

			c.configMu.RLock()
			seed := c.config.ClientID
			c.configMu.RUnlock()

			pH := c.hash(seed, prev)
			nH := c.hash(seed, now)
			c.logger.Debug("Alive nodes delta", zap.Int("prevNum", len(prev)), zap.Uint64("prevHash", pH), zap.Int("currNum", len(now)), zap.Uint64("currHash", nH))

			if failed > 0 || pH != nH {
				c.logger.Info("Connections with specter server have changed", zap.Int("previous", len(prev)), zap.Int("current", len(now)))
				c.SyncConfigTunnels(ctx)
			}
		}
	}
}

func (c *Client) maintainConnections(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()

	nodes, err := c.RequestCandidates(callCtx)
	if err != nil {
		return err
	}

	c.logger.Debug("Candidates for RPC connections", zap.Int("num", len(nodes)))
	for _, node := range nodes {
		err := func() error {
			if err := c.openRPC(ctx, node); err != nil {
				return fmt.Errorf("connecting to specter server: %w", err)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) Register(ctx context.Context) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	if c.config.Token != "" {
		req := &protocol.ClientRequest{
			Kind: protocol.TunnelRPC_PING,
			Token: &protocol.ClientToken{
				Token: []byte(c.config.Token),
			},
		}
		resp, err := c.rpcCall(ctx, req)
		if err != nil {
			return err
		}
		root := resp.GetPingResponse().GetApex()
		c.rootDomain.Store(root)

		c.logger.Info("Reusing existing client token")

		return nil
	}

	req := &protocol.ClientRequest{
		Kind: protocol.TunnelRPC_IDENTITY,
		RegisterRequest: &protocol.RegisterIdentityRequest{
			Client: c.serverTransport.Identity(),
		},
	}
	resp, err := c.rpcCall(ctx, req)
	if err != nil {
		return err
	}
	token := resp.GetRegisterResponse().GetToken().GetToken()
	root := resp.GetRegisterResponse().GetApex()
	c.rootDomain.Store(root)
	c.config.Token = string(token)

	c.logger.Info("Client token obtained via apex")

	if err := c.config.writeFile(); err != nil {
		c.logger.Error("Error saving token to config file", zap.Error(err))
	}
	return nil
}

func (c *Client) Ping(ctx context.Context, rpc rpc.RPC) (*protocol.ClientPingResponse, error) {
	req := &protocol.RPC_Request{
		Kind: protocol.RPC_CLIENT_REQUEST,
		ClientRequest: &protocol.ClientRequest{
			Kind: protocol.TunnelRPC_PING,
		},
	}
	resp, err := rpc.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetClientResponse().GetPingResponse(), nil
}

func (c *Client) RequestHostname(ctx context.Context) (string, error) {
	req := &protocol.ClientRequest{
		Kind:  protocol.TunnelRPC_HOSTNAME,
		Token: c.getToken(),
		RegisterRequest: &protocol.RegisterIdentityRequest{
			Client: c.serverTransport.Identity(),
		},
	}
	resp, err := c.rpcCall(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.GetHostnameResponse().GetHostname(), nil
}

func (c *Client) RequestCandidates(ctx context.Context) ([]*protocol.Node, error) {
	req := &protocol.ClientRequest{
		Kind:         protocol.TunnelRPC_NODES,
		Token:        c.getToken(),
		NodesRequest: &protocol.GetNodesRequest{},
	}
	resp, err := c.rpcCall(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetNodesResponse().GetNodes(), nil
}

func (c *Client) SyncConfigTunnels(ctx context.Context) {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()

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

	connected := c.getConnectedNodes(ctx)
	apex := c.rootDomain.Load()

	for _, tunnel := range tunnels {
		if tunnel.Hostname == "" {
			continue
		}
		published, err := c.PublishTunnel(ctx, tunnel.Hostname, connected)
		if err != nil {
			c.logger.Error("Failed to publish tunnel", zap.String("hostname", tunnel.Hostname), zap.String("target", tunnel.Target), zap.Int("endpoints", len(connected)), zap.Error(err))
			continue
		}
		c.logger.Info("Tunnel published", zap.String("hostname", fmt.Sprintf("%s.%s", tunnel.Hostname, apex)), zap.String("target", tunnel.Target), zap.Int("published", len(published)))
	}

	c.configMu.Lock()
	c.config.Tunnels = tunnels
	if err := c.config.writeFile(); err != nil {
		c.logger.Error("Error saving token to config file", zap.Error(err))
	}
	c.config.buildRouter()
	c.configMu.Unlock()
}

func (c *Client) PublishTunnel(ctx context.Context, hostname string, connected []*protocol.Node) ([]*protocol.Node, error) {
	req := &protocol.ClientRequest{
		Kind:  protocol.TunnelRPC_TUNNEL,
		Token: c.getToken(),
		TunnelRequest: &protocol.PublishTunnelRequest{
			Hostname: hostname,
			Servers:  connected,
		},
	}
	resp, err := c.rpcCall(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetTunnelResponse().GetPublished(), nil
}

func (c *Client) reloadOnSignal(ctx context.Context) {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s:
			c.configMu.Lock()
			if err := c.config.reloadFile(); err != nil {
				c.logger.Error("Error reloading config file", zap.Error(err))
				c.configMu.Unlock()
				continue
			}
			c.configMu.Unlock()
			c.SyncConfigTunnels(ctx)
		}
	}
}

func (c *Client) Accept(ctx context.Context) {
	c.logger.Info("Listening for tunnel traffic")

	go c.periodicReconnection(ctx)
	go c.reloadOnSignal(ctx)

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

			c.logger.Info("Incoming connection from gateway",
				zap.String("protocol", link.Alpn.String()),
				zap.String("hostname", link.Hostname),
				zap.String("remote", link.Remote))

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

func (c *Client) Close() {
	c.proxies.Range(func(key string, proxy *httpProxy) bool {
		c.logger.Info("Shutting down proxy", zap.String("hostname", key))
		proxy.acceptor.Close()
		return true
	})
	c.connections.Range(func(key uint64, conn *connection) bool {
		c.logger.Info("Closing connection with specter server", zap.String("addr", conn.peer.GetAddress()))
		conn.rpc.Close()
		return true
	})
}

func (c *Client) forward(ctx context.Context, remote net.Conn, dest string) {
	dialer := &net.Dialer{
		Timeout: time.Second * 3,
	}
	local, err := dialer.DialContext(ctx, "tcp", dest)
	if err != nil {
		c.logger.Error("forwarding connection", zap.Error(err))
		tun.SendStatusProto(remote, err)
		remote.Close()
		return
	}
	tun.SendStatusProto(remote, nil)
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
			if errors.Is(e, context.Canceled) ||
				errors.Is(e, io.EOF) {
				// this is expected
				return
			}
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
