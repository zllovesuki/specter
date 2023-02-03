package client

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/maphash"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/rtt"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util"
	"kon.nect.sh/specter/util/acceptor"
	"kon.nect.sh/specter/util/pipe"

	"github.com/avast/retry-go/v4"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var (
	checkInterval = time.Second * 30
	hashSeed      = maphash.MakeSeed()
)

const (
	connectTimeout = time.Second * 5
	rpcTimeout     = time.Second * 5
)

type ClientConfig struct {
	Logger           *zap.Logger
	Configuration    *Config
	ServerTransport  transport.Transport
	Recorder         rtt.Recorder
	ReloadSignal     <-chan os.Signal
	DisableTargetTLS bool
}

type Client struct {
	configMu     sync.RWMutex
	closeWg      sync.WaitGroup
	syncMu       sync.Mutex
	tunnelClient protocol.TunnelService
	parentCtx    context.Context
	rootDomain   *atomic.String
	proxies      *skipmap.StringMap[*httpProxy]
	connections  *skipmap.Uint64Map[*protocol.Node]
	closeCh      chan struct{}
	closed       atomic.Bool
	ClientConfig
}

func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	cfg.Logger = cfg.Logger.With(zap.Uint64("id", cfg.ServerTransport.Identity().GetId()))
	c := &Client{
		ClientConfig: cfg,
		parentCtx:    ctx,
		rootDomain:   atomic.NewString(""),
		proxies:      skipmap.NewString[*httpProxy](),
		connections:  skipmap.NewUint64[*protocol.Node](),
		tunnelClient: rpc.DynamicTunnelClient(ctx, cfg.ServerTransport),
		closeCh:      make(chan struct{}),
	}

	if err := c.openRPC(ctx, &protocol.Node{
		Address: cfg.Configuration.Apex,
	}); err != nil {
		return nil, fmt.Errorf("error connecting to apex: %w", err)
	}

	return c, nil
}

func (c *Client) Initialize(ctx context.Context) error {
	if err := c.maintainConnections(ctx); err != nil {
		return err
	}

	c.Logger.Info("Waiting for RTT measurement...", zap.Duration("max", transport.RTTMeasureInterval))
	time.Sleep(util.RandomTimeRange(transport.RTTMeasureInterval))

	c.SyncConfigTunnels(ctx)
	return nil
}

func (c *Client) openRPC(ctx context.Context, node *protocol.Node) error {
	if _, ok := c.connections.Load(node.GetId()); ok {
		return nil
	}

	callCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	resp, err := c.Ping(callCtx, node)
	if err != nil {
		return err
	}

	identity := resp.GetNode()
	c.Logger.Info("Connected to specter server", zap.String("addr", identity.GetAddress()))
	c.connections.Store(identity.GetId(), identity)
	return nil
}

func retryRPC[V any](c *Client, ctx context.Context, fn func(node *protocol.Node) (V, error)) (resp V, err error) {
	err = retry.Do(func() error {
		var (
			candidate *protocol.Node
			rpcError  error
		)
		c.connections.Range(func(id uint64, node *protocol.Node) bool {
			candidate = node
			return false
		})
		if candidate == nil {
			return fmt.Errorf("no rpc candidates available")
		}
		resp, rpcError = fn(candidate)
		return chord.ErrorMapper(rpcError)
	},
		retry.Context(ctx),
		retry.Attempts(2),
		retry.LastErrorOnly(true),
		retry.Delay(time.Millisecond*500),
		retry.RetryIf(chord.ErrorIsRetryable),
	)
	return
}

func (c *Client) getConnectedNodes(ctx context.Context) []*protocol.Node {
	nodes := make([]*protocol.Node, 0)
	c.connections.Range(func(id uint64, node *protocol.Node) bool {
		if len(nodes) < tun.NumRedundantLinks {
			nodes = append(nodes, node)
		}
		return true
	})
	return nodes
}

func (c *Client) getAliveNodes(ctx context.Context) (alive []*protocol.Node, dead int) {
	alive = make([]*protocol.Node, 0)
	c.connections.Range(func(id uint64, node *protocol.Node) bool {
		func() {
			callCtx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()

			_, err := c.Ping(callCtx, node)
			if err != nil {
				c.connections.Delete(id)
				dead++
			} else {
				alive = append(alive, node)
			}
		}()
		return true
	})
	return
}

func (c *Client) getToken() *protocol.ClientToken {
	c.configMu.RLock()
	token := c.Configuration.Token
	c.configMu.RUnlock()
	return &protocol.ClientToken{
		Token: []byte(token),
	}
}

func (c *Client) hash(seed uint64, nodes []*protocol.Node) uint64 {
	var buf [8]byte

	hasher := maphash.Hash{}
	hasher.SetSeed(hashSeed)

	for _, node := range nodes {
		binary.BigEndian.PutUint64(buf[:], node.GetId())
		hasher.Write(buf[:])
	}
	return hasher.Sum64()
}

func (c *Client) periodicReconnection(ctx context.Context) {
	defer c.closeWg.Done()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			prev, failed := c.getAliveNodes(ctx)
			if failed > 0 {
				c.Logger.Info("Some connections have failed, opening more connections to specter server", zap.Int("dead", failed))
			}
			if err := c.maintainConnections(ctx); err != nil {
				continue
			}
			now, _ := c.getAliveNodes(ctx)

			c.configMu.RLock()
			seed := c.Configuration.ClientID
			c.configMu.RUnlock()

			pH := c.hash(seed, prev)
			nH := c.hash(seed, now)
			c.Logger.Debug("Alive nodes delta", zap.Int("prevNum", len(prev)), zap.Uint64("prevHash", pH), zap.Int("currNum", len(now)), zap.Uint64("currHash", nH))

			if failed > 0 || pH != nH {
				c.Logger.Info("Connections with specter server have changed", zap.Int("previous", len(prev)), zap.Int("current", len(now)))
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
	c.Logger.Debug("Candidates for RPC connections", zap.Int("num", len(nodes)))

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

	if c.Configuration.Token != "" {
		resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.ClientPingResponse, error) {
			ctx = rpc.WithClientToken(ctx, &protocol.ClientToken{
				Token: []byte(c.Configuration.Token),
			})
			ctx = rpc.WithNode(ctx, node)
			return c.tunnelClient.Ping(ctx, &protocol.ClientPingRequest{})
		})
		if err != nil {
			return err
		}
		root := resp.GetApex()
		c.rootDomain.Store(root)

		c.Logger.Info("Reusing existing client token")

		return nil
	}

	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.RegisterIdentityResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.RegisterIdentity(ctx, &protocol.RegisterIdentityRequest{
			Client: c.ServerTransport.Identity(),
		})
	})
	if err != nil {
		return err
	}

	token := resp.GetToken().GetToken()
	root := resp.GetApex()
	c.rootDomain.Store(root)
	c.Configuration.Token = string(token)

	c.Logger.Info("Client token obtained via apex")

	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving token to config file", zap.Error(err))
	}
	return nil
}

func (c *Client) Ping(ctx context.Context, node *protocol.Node) (*protocol.ClientPingResponse, error) {
	return c.tunnelClient.Ping(rpc.WithNode(ctx, node), &protocol.ClientPingRequest{})
}

func (c *Client) RequestHostname(ctx context.Context) (string, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.GenerateHostnameResponse, error) {
		ctx = rpc.WithClientToken(ctx, c.getToken())
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.GenerateHostname(ctx, &protocol.GenerateHostnameRequest{})
	})
	if err != nil {
		return "", err
	}
	return resp.GetHostname(), nil
}

func (c *Client) RequestCandidates(ctx context.Context) ([]*protocol.Node, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.GetNodesResponse, error) {
		ctx = rpc.WithClientToken(ctx, c.getToken())
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.GetNodes(ctx, &protocol.GetNodesRequest{})
	})
	if err != nil {
		return nil, err
	}
	return resp.GetNodes(), nil
}

func (c *Client) SyncConfigTunnels(ctx context.Context) {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()

	c.configMu.RLock()
	tunnels := append([]Tunnel{}, c.Configuration.Tunnels...)
	c.configMu.RUnlock()

	c.Logger.Info("Synchronizing tunnels in config file with specter", zap.Int("tunnels", len(tunnels)))

	for i, tunnel := range tunnels {
		if tunnel.Hostname == "" && tunnel.Target != "" {
			name, err := c.RequestHostname(ctx)
			if err != nil {
				c.Logger.Error("Failed to request hostname", zap.String("target", tunnel.Target), zap.Error(err))
				continue
			}
			tunnels[i].Hostname = name
		}
	}

	connected := c.getConnectedNodes(ctx)
	apex := c.rootDomain.Load()

	rttLookup := make(map[string]time.Duration)
	for _, n := range connected {
		m := c.Recorder.Snapshot(rtt.MakeMeasurementKey(n), time.Second*10)
		if m == nil {
			continue
		}
		rttLookup[rtt.MakeMeasurementKey(n)] = m.Average
	}
	sort.SliceStable(connected, func(i, j int) bool {
		l, lOK := rttLookup[rtt.MakeMeasurementKey(connected[i])]
		r, rOK := rttLookup[rtt.MakeMeasurementKey(connected[j])]
		if lOK && !rOK {
			return true
		}
		if !lOK && rOK {
			return false
		}
		return l < r
	})

	c.Logger.Debug("rtt information", zap.String("table", fmt.Sprint(rttLookup)))

	for _, tunnel := range tunnels {
		if tunnel.Hostname == "" {
			continue
		}
		published, err := c.PublishTunnel(ctx, tunnel.Hostname, connected)
		if err != nil {
			c.Logger.Error("Failed to publish tunnel", zap.String("hostname", tunnel.Hostname), zap.String("target", tunnel.Target), zap.Int("endpoints", len(connected)), zap.Error(err))
			continue
		}
		c.Logger.Info("Tunnel published", zap.String("hostname", fmt.Sprintf("%s.%s", tunnel.Hostname, apex)), zap.String("target", tunnel.Target), zap.Int("published", len(published)))
	}

	c.configMu.Lock()
	c.Configuration.Tunnels = tunnels
	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving token to config file", zap.Error(err))
	}
	c.Configuration.buildRouter()
	c.configMu.Unlock()
}

func (c *Client) PublishTunnel(ctx context.Context, hostname string, connected []*protocol.Node) ([]*protocol.Node, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.PublishTunnelResponse, error) {
		ctx = rpc.WithClientToken(ctx, c.getToken())
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.PublishTunnel(ctx, &protocol.PublishTunnelRequest{
			Hostname: hostname,
			Servers:  connected,
		})
	})
	if err != nil {
		return nil, err
	}
	return resp.GetPublished(), nil
}

func (c *Client) reloadOnSignal(ctx context.Context) {
	defer c.closeWg.Done()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ctx.Done():
			return
		case <-c.ReloadSignal:
			c.Logger.Info("Received SIGHUP, reloading config")
			c.configMu.Lock()
			if err := c.Configuration.reloadFile(); err != nil {
				c.Logger.Error("Error reloading config file", zap.Error(err))
				c.configMu.Unlock()
				continue
			}
			c.configMu.Unlock()
			c.SyncConfigTunnels(ctx)
		}
	}
}

func (c *Client) Start(ctx context.Context) {
	c.Logger.Info("Listening for tunnel traffic")

	c.closeWg.Add(2)

	streamRouter := transport.NewStreamRouter(c.Logger, nil, c.ServerTransport)
	streamRouter.HandleTunnel(protocol.Stream_DIRECT, func(delegation *transport.StreamDelegate) {
		link := &protocol.Link{}
		if err := rpc.Receive(delegation, link); err != nil {
			c.Logger.Error("Receiving link information from gateway", zap.Error(err))
			delegation.Close()
			return
		}
		hostname := link.Hostname
		u, ok := c.Configuration.router.Load(hostname)
		if !ok {
			c.Logger.Error("Unknown hostname in connection", zap.String("hostname", hostname))
			delegation.Close()
			return
		}

		c.Logger.Info("Incoming connection from gateway",
			zap.String("protocol", link.Alpn.String()),
			zap.String("hostname", link.Hostname),
			zap.String("remote", link.Remote))

		switch link.GetAlpn() {
		case protocol.Link_HTTP:
			c.getHTTPProxy(ctx, hostname, u).acceptor.Handle(delegation)

		case protocol.Link_TCP:
			c.forwardStream(ctx, delegation, u)

		default:
			c.Logger.Error("Unknown alpn for forwarding", zap.String("alpn", link.GetAlpn().String()))
			delegation.Close()
		}
	})

	go streamRouter.Accept(ctx)
	go c.periodicReconnection(ctx)
	go c.reloadOnSignal(ctx)
}

func (c *Client) Close() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	c.proxies.Range(func(key string, proxy *httpProxy) bool {
		c.Logger.Info("Shutting down proxy", zap.String("hostname", key))
		proxy.acceptor.Close()
		return true
	})
	close(c.closeCh)
	c.closeWg.Wait()
}

func (c *Client) forwardStream(ctx context.Context, remote net.Conn, u *url.URL) {
	var (
		target string
		local  net.Conn
		err    error
	)
	switch u.Scheme {
	case "tcp":
		dialer := &net.Dialer{
			Timeout: time.Second * 3,
		}
		target = u.Host
		local, err = dialer.DialContext(ctx, "tcp", u.Host)
	case "unix", "winio":
		target = u.Path
		local, err = pipe.DialPipe(ctx, u.Path)
	default:
		err = fmt.Errorf("unknown scheme: %s", u.Scheme)
	}
	if err != nil {
		c.Logger.Error("Error dialing to target", zap.String("target", target), zap.Error(err))
		tun.SendStatusProto(remote, err)
		remote.Close()
		return
	}
	tun.SendStatusProto(remote, nil)
	tun.Pipe(remote, local)
}

func (c *Client) getHTTPProxy(ctx context.Context, hostname string, u *url.URL) *httpProxy {
	proxy, loaded := c.proxies.LoadOrStoreLazy(hostname, func() *httpProxy {
		c.Logger.Info("Creating new proxy", zap.String("hostname", hostname))

		tp := &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName:         u.Host,
				InsecureSkipVerify: c.DisableTargetTLS,
			},
			MaxIdleConns:    10,
			IdleConnTimeout: time.Minute,
		}

		isPipe := false
		switch u.Scheme {
		case "unix", "winio":
			isPipe = true
			tp.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return pipe.DialPipe(ctx, u.Path)
			}
		}
		proxy := httputil.NewSingleHostReverseProxy(u)
		d := proxy.Director
		// https://stackoverflow.com/a/53007606
		// need to overwrite Host field
		proxy.Director = func(r *http.Request) {
			d(r)
			if isPipe {
				r.Host = "pipe"
				r.URL.Host = "pipe"
				r.URL.Scheme = "http"
				r.URL.Path = strings.ReplaceAll(r.URL.Path, u.Path, "")
			} else {
				r.Host = u.Host
			}
		}
		proxy.Transport = tp
		proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, e error) {
			if errors.Is(e, context.Canceled) ||
				errors.Is(e, io.EOF) {
				// this is expected
				return
			}
			c.Logger.Error("Error forwarding http/https request", zap.Error(e))
			rw.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(rw, "Forwarding target returned error: %s", e.Error())
		}
		proxy.ErrorLog = zap.NewStdLog(c.Logger)

		accepter := acceptor.NewH2Acceptor(nil)
		h2s := &http2.Server{}
		forwarder := &http.Server{
			Handler:           h2c.NewHandler(proxy, h2s),
			ErrorLog:          zap.NewStdLog(c.Logger),
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
