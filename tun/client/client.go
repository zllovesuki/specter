package client

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	pkiImpl "go.miragespace.co/specter/pki"
	"go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/pow"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util"
	"go.miragespace.co/specter/util/acceptor"

	"github.com/avast/retry-go/v4"
	"github.com/zeebo/xxh3"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

var (
	checkInterval = time.Second * 30
	rttInterval   = transport.RTTMeasureInterval
)

const (
	connectTimeout = time.Second * 5
	rpcTimeout     = time.Second * 5
)

type ClientConfig struct {
	Logger          *zap.Logger
	Configuration   *Config
	PKIClient       protocol.PKIService
	ServerTransport transport.Transport
	Recorder        rtt.Recorder
	ReloadSignal    <-chan os.Signal
	ServerListener  net.Listener
}

type Client struct {
	ClientConfig
	configMu     sync.RWMutex
	closeWg      sync.WaitGroup
	syncMu       sync.Mutex
	tunnelClient protocol.TunnelService
	parentCtx    context.Context
	rootDomain   *atomic.String
	proxies      *skipmap.StringMap[*httpProxy]
	connections  *skipmap.StringMap[*protocol.Node]
	rpcAcceptor  *acceptor.HTTP2Acceptor
	closeCh      chan struct{}
	closed       atomic.Bool
}

func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	c := &Client{
		ClientConfig: cfg,
		parentCtx:    ctx,
		rootDomain:   atomic.NewString(""),
		proxies:      skipmap.NewString[*httpProxy](),
		connections:  skipmap.NewString[*protocol.Node](),
		tunnelClient: rpc.DynamicTunnelClient(ctx, cfg.ServerTransport),
		rpcAcceptor:  acceptor.NewH2Acceptor(nil),
		closeCh:      make(chan struct{}),
	}

	if c.Configuration.Certificate != "" {
		c.configMu.RLock()
		apex := c.ClientConfig.Configuration.Apex
		c.configMu.RUnlock()
		if err := c.updateTransportCert(); err != nil {
			return nil, err
		}
		if err := c.bootstrap(ctx, apex); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *Client) Initialize(ctx context.Context, syncTunnels bool) error {
	if err := c.maintainConnections(ctx); err != nil {
		return err
	}

	c.Logger.Info("Waiting for RTT measurement...", zap.Duration("max", rttInterval))
	time.Sleep(util.RandomTimeRange(rttInterval))

	if syncTunnels {
		c.SyncConfigTunnels(ctx)
	}
	return nil
}

func (c *Client) updateTransportCert() error {
	cert, err := tls.X509KeyPair([]byte(c.Configuration.Certificate), []byte(c.Configuration.PrivKey))
	if err != nil {
		return fmt.Errorf("error parsing certificate: %w", err)
	}
	if tp, ok := c.ServerTransport.(transport.ClientTransport); ok {
		if err := tp.WithClientCertificate(cert); err != nil {
			return err
		}
		c.Logger = c.Logger.With(zap.Uint64("id", c.ServerTransport.Identity().GetId()))
	} else {
		return fmt.Errorf("transport does not support client certificate override")
	}
	return nil
}

func (c *Client) bootstrap(ctx context.Context, apex string) error {
	return c.openRPC(ctx, &protocol.Node{
		Address: apex,
	})
}

func (c *Client) openRPC(ctx context.Context, node *protocol.Node) error {
	if _, ok := c.connections.Load(node.GetAddress()); ok {
		return nil
	}

	callCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	resp, err := c.ping(callCtx, node)
	if err != nil {
		return err
	}

	identity := resp.GetNode()
	c.Logger.Info("Connected to specter server", zap.String("addr", identity.GetAddress()))
	c.connections.Store(identity.GetAddress(), identity)

	return nil
}

func retryRPC[V any](c *Client, ctx context.Context, fn func(node *protocol.Node) (V, error)) (resp V, err error) {
	candidates := c.getConnectedNodes()
	err = retry.Do(func() error {
		var (
			candidate *protocol.Node
			rpcError  error
		)
		if len(candidates) > 0 {
			candidate, candidates = candidates[0], candidates[1:]
		}
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

func (c *Client) getConnectedNodes() (nodes []*protocol.Node) {
	c.connections.Range(func(_ string, node *protocol.Node) bool {
		if len(nodes) < tun.NumRedundantLinks {
			nodes = append(nodes, node)
		}
		return true
	})

	// fast path exit if we don't have rtt enabled
	if c.Recorder == nil {
		return
	}

	// sort routes based on rtt to different gateways, so hostname/1 and rpc calls
	// always resolves to the gateway with the lowest rtt to the client
	rttLookup := make(map[string]time.Duration)
	for _, n := range nodes {
		m := c.Recorder.Snapshot(rtt.MakeMeasurementKey(n), time.Second*10)
		if m == nil {
			continue
		}
		rttLookup[rtt.MakeMeasurementKey(n)] = m.Average
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		l, lOK := rttLookup[rtt.MakeMeasurementKey(nodes[i])]
		r, rOK := rttLookup[rtt.MakeMeasurementKey(nodes[j])]
		if lOK && !rOK {
			return true
		}
		if !lOK && rOK {
			return false
		}
		return l < r
	})

	c.Logger.Debug("rtt information", zap.String("table", fmt.Sprint(rttLookup)))

	return nodes
}

func (c *Client) getAliveNodes(ctx context.Context) (alive []*protocol.Node, dead int) {
	alive = make([]*protocol.Node, 0)
	c.connections.Range(func(addr string, node *protocol.Node) bool {
		func() {
			callCtx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()

			_, err := c.ping(callCtx, node)
			if err != nil {
				c.connections.Delete(addr)
				dead++
			} else {
				alive = append(alive, node)
			}
		}()
		return true
	})
	return
}

func (c *Client) hash(seed uint64, nodes []*protocol.Node) uint64 {
	var buf [8]byte

	hasher := xxh3.New()

	for _, node := range nodes {
		binary.BigEndian.PutUint64(buf[:], node.GetId())
		hasher.Write(buf[:])
	}
	return hasher.Sum64()
}

func (c *Client) reBootstrap(ctx context.Context) {
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
			connected := c.getConnectedNodes()
			if len(connected) > 0 {
				continue
			}
			c.Logger.Info("No connected nodes, re-bootstrapping using apex")
			c.configMu.RLock()
			apex := c.ClientConfig.Configuration.Apex
			c.configMu.RUnlock()
			if err := c.bootstrap(ctx, apex); err != nil {
				c.Logger.Error("Failed to rebootstrap connection to specter", zap.Error(err))
			}
		}
	}
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

			var seed uint64 = 0
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

	nodes, err := c.requestCandidates(callCtx)
	if err != nil {
		return err
	}
	c.Logger.Debug("Candidates for RPC connections", zap.Int("num", len(nodes)))

	for _, node := range nodes {
		if c.connections.Len() >= tun.NumRedundantLinks {
			return nil
		}
		if err := c.openRPC(ctx, node); err != nil {
			return fmt.Errorf("connecting to specter server: %w", err)
		}
	}

	return nil
}

func (c *Client) Register(ctx context.Context) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	if c.Configuration.Certificate != "" {
		clientCert, err := tls.X509KeyPair([]byte(c.Configuration.Certificate), []byte(c.Configuration.PrivKey))
		if err != nil {
			return fmt.Errorf("error parsing certificate: %w", err)
		}
		cert, err := x509.ParseCertificate(clientCert.Certificate[0])
		if err != nil {
			return fmt.Errorf("error parsing certificate: %w", err)
		}

		resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.ClientPingResponse, error) {
			ctx = rpc.WithNode(ctx, node)
			return c.tunnelClient.Ping(ctx, &protocol.ClientPingRequest{})
		})
		if err != nil {
			return err
		}
		root := resp.GetApex()
		c.rootDomain.Store(root)

		identity, err := pki.ExtractCertificateIdentity(cert)
		if err != nil {
			return fmt.Errorf("failed to extract certificate identity: %w", err)
		}
		c.Logger.Info("Reusing existing client certificate", zap.Object("identity", identity))

		return nil
	}

	if c.PKIClient == nil {
		return errors.New("no client certificate found: please ensure your client is registered with apex first with the tunnel subcommand")
	}

	c.Logger.Info("Obtaining a new client certificate from apex")

	privKey, err := pki.UnmarshalPrivateKey([]byte(c.Configuration.PrivKey))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	pkiReq, err := pkiImpl.CreateRequest(privKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate request: %w", err)
	}

	pkiResp, err := c.PKIClient.RequestCertificate(c.parentCtx, pkiReq)
	if err != nil {
		return fmt.Errorf("failed to obtain a client certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(pkiResp.GetCertDer())
	if err != nil {
		return fmt.Errorf("invalid certificate from PKIService: %w", err)
	}

	c.Configuration.Certificate = string(pkiResp.GetCertPem())

	identity, err := pki.ExtractCertificateIdentity(cert)
	if err != nil {
		return fmt.Errorf("failed to extract certificate identity: %w", err)
	}
	c.Logger.Info("Client certificate obtained", zap.Object("identity", identity))

	if err := c.updateTransportCert(); err != nil {
		return fmt.Errorf("failed to update transport certificate: %w", err)
	}

	if err := c.bootstrap(c.parentCtx, c.Configuration.Apex); err != nil {
		return fmt.Errorf("failed to bootstrap with certificate: %w", err)
	}

	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.RegisterIdentityResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.RegisterIdentity(ctx, &protocol.RegisterIdentityRequest{})
	})
	if err != nil {
		return err
	}

	root := resp.GetApex()
	c.rootDomain.Store(root)

	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving token to config file", zap.Error(err))
	}

	return nil
}

func (c *Client) ping(ctx context.Context, node *protocol.Node) (*protocol.ClientPingResponse, error) {
	return c.tunnelClient.Ping(rpc.WithNode(ctx, node), &protocol.ClientPingRequest{})
}

func (c *Client) requestHostname(ctx context.Context) (string, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.GenerateHostnameResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.GenerateHostname(ctx, &protocol.GenerateHostnameRequest{})
	})
	if err != nil {
		return "", err
	}
	return resp.GetHostname(), nil
}

func (c *Client) requestCandidates(ctx context.Context) ([]*protocol.Node, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.GetNodesResponse, error) {
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

	// reuse already assigned hostnames if possible
	registered, err := c.GetRegisteredHostnames(ctx)
	if err != nil {
		c.Logger.Error("Failed to query available hostnames", zap.Error(err))
		return
	}
	available := make([]string, 0)
	inused := make(map[string]string)
	for _, t := range tunnels {
		inused[t.Hostname] = t.Target
	}
	for _, hostname := range registered {
		// while we want to reuse hostnames, we want to reuse auto-generated hostnames only
		// so we don't accidentally point, say, pointing bastion.customdomain.com to MySQL
		if strings.Contains(hostname, ".") {
			continue
		}
		// filter out hostnames currently in used
		if _, ok := inused[hostname]; ok {
			continue
		}
		available = append(available, hostname)
	}

	// now assign a hostname to a target if they don't have one, either a new hostname or reused
	var name string
	for i, tunnel := range tunnels {
		if tunnel.Target == "" {
			continue
		}
		if tunnel.Hostname == "" {
			if len(available) > 0 {
				name, available = available[0], available[1:]
			} else {
				name, err = c.requestHostname(ctx)
				if err != nil {
					c.Logger.Error("Failed to request new hostname", zap.String("target", tunnel.Target), zap.Error(err))
					continue
				}
			}
			tunnels[i].Hostname = name
		}
		// TODO: assert that the hostname was assigned
	}

	connected := c.getConnectedNodes()
	apex := c.rootDomain.Load()

	for _, tunnel := range tunnels {
		if tunnel.Hostname == "" {
			continue
		}
		published, err := c.publishTunnel(ctx, tunnel.Hostname, connected)
		if err != nil {
			c.Logger.Error("Failed to publish tunnel", zap.String("hostname", tunnel.Hostname), zap.String("target", tunnel.Target), zap.Int("endpoints", len(connected)), zap.Error(err))
			continue
		}

		var fqdn string
		if strings.Contains(tunnel.Hostname, ".") {
			fqdn = tunnel.Hostname
		} else {
			fqdn = fmt.Sprintf("%s.%s", tunnel.Hostname, apex)
		}
		c.Logger.Info("Tunnel published", zap.String("hostname", fqdn), zap.String("target", tunnel.Target), zap.Int("published", len(published)))
	}

	c.RebuildTunnels(tunnels)
}

func (c *Client) publishTunnel(ctx context.Context, hostname string, connected []*protocol.Node) ([]*protocol.Node, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.PublishTunnelResponse, error) {
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
			c.doReload(ctx)
		}
	}
}

func (c *Client) doReload(ctx context.Context) {
	onReload := func(prev, curr []Tunnel) {
		diff := diffTunnels(prev, curr)
		c.closeOutdatedProxies(diff...)
		c.Configuration.buildRouter(diff...)
	}
	c.configMu.Lock()
	if err := c.Configuration.reloadFile(onReload); err != nil {
		c.Logger.Error("Error reloading config file", zap.Error(err))
		c.configMu.Unlock()
		return
	}
	c.configMu.Unlock()
	c.SyncConfigTunnels(ctx)
}

func (c *Client) GetRegisteredHostnames(ctx context.Context) ([]string, error) {
	resp, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.RegisteredHostnamesResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.RegisteredHostnames(ctx, &protocol.RegisteredHostnamesRequest{})
	})
	if err != nil {
		return nil, err
	}
	return resp.GetHostnames(), nil
}

func (c *Client) obtainAcmeProof(hostname string) (*protocol.ProofOfWork, error) {
	c.configMu.RLock()
	privKey, err := pki.UnmarshalPrivateKey([]byte(c.Configuration.PrivKey))
	c.configMu.RUnlock()
	if err != nil {
		return nil, err
	}

	return pow.GenerateSolution(privKey, pow.Parameters{
		Difficulty: acme.HashcashDifficulty,
		Expires:    acme.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			return hostname
		},
	})
}

func (c *Client) GetAcmeInstruction(ctx context.Context, hostname string) (*protocol.InstructionResponse, error) {
	proof, err := c.obtainAcmeProof(hostname)
	if err != nil {
		return nil, err
	}

	return retryRPC(c, ctx, func(node *protocol.Node) (*protocol.InstructionResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.AcmeInstruction(ctx, &protocol.InstructionRequest{
			Proof:    proof,
			Hostname: hostname,
		})
	})
}

func (c *Client) RequestAcmeValidation(ctx context.Context, hostname string) (*protocol.ValidateResponse, error) {
	proof, err := c.obtainAcmeProof(hostname)
	if err != nil {
		return nil, err
	}

	return retryRPC(c, ctx, func(node *protocol.Node) (*protocol.ValidateResponse, error) {
		ctx = rpc.WithNode(ctx, node)
		return c.tunnelClient.AcmeValidate(ctx, &protocol.ValidateRequest{
			Proof:    proof,
			Hostname: hostname,
		})
	})
}

func (c *Client) GetConnectedNodes() []*protocol.Node {
	return c.getConnectedNodes()
}

func (c *Client) GetCurrentConfig() *Config {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	return c.Configuration.clone()
}

func (c *Client) RebuildTunnels(tunnels []Tunnel) {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	diff := diffTunnels(c.Configuration.Tunnels, tunnels)
	c.closeOutdatedProxies(diff...)

	c.Configuration.Tunnels = tunnels
	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving to config file", zap.Error(err))
	}

	c.Configuration.validate()
	c.Configuration.buildRouter(diff...)
}

func (c *Client) tunnelRemovalWrapper(tunnel Tunnel, fn func() error) error {
	if err := fn(); err != nil {
		return err
	}

	c.configMu.Lock()
	defer c.configMu.Unlock()

	var index int = -1
	for i, t := range c.Configuration.Tunnels {
		if t.Hostname == tunnel.Hostname {
			index = i
			break
		}
	}
	if index == -1 {
		return nil
	}

	c.closeOutdatedProxies(tunnel)

	c.Configuration.Tunnels = append(c.Configuration.Tunnels[:index], c.Configuration.Tunnels[index+1:]...)
	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving to config file", zap.Error(err))
	}

	c.Configuration.validate()
	c.Configuration.buildRouter(tunnel)

	return nil
}

func (c *Client) UnpublishTunnel(ctx context.Context, tunnel Tunnel) error {
	err := c.tunnelRemovalWrapper(tunnel, func() error {
		_, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.UnpublishTunnelResponse, error) {
			ctx = rpc.WithNode(ctx, node)
			return c.tunnelClient.UnpublishTunnel(ctx, &protocol.UnpublishTunnelRequest{
				Hostname: tunnel.Hostname,
			})
		})
		return err
	})
	return err
}

func (c *Client) ReleaseTunnel(ctx context.Context, tunnel Tunnel) error {
	return c.tunnelRemovalWrapper(tunnel, func() error {
		_, err := retryRPC(c, ctx, func(node *protocol.Node) (*protocol.ReleaseTunnelResponse, error) {
			ctx = rpc.WithNode(ctx, node)
			return c.tunnelClient.ReleaseTunnel(ctx, &protocol.ReleaseTunnelRequest{
				Hostname: tunnel.Hostname,
			})
		})
		return err
	})
}

func (c *Client) UpdateApex(apex string) {
	c.configMu.Lock()
	c.Configuration.Apex = apex
	if err := c.Configuration.writeFile(); err != nil {
		c.Logger.Error("Error saving to config file", zap.Error(err))
	}
	c.configMu.Unlock()
}

func (c *Client) Start(ctx context.Context) {
	c.Logger.Info("Listening for tunnel traffic")

	c.closeWg.Add(3)

	streamRouter := transport.NewStreamRouter(c.Logger, nil, c.ServerTransport)
	streamRouter.HandleTunnel(protocol.Stream_DIRECT, func(delegation *transport.StreamDelegate) {
		link := &protocol.Link{}
		if err := rpc.Receive(delegation, link); err != nil {
			c.Logger.Error("Receiving link information from gateway", zap.Error(err))
			delegation.Close()
			return
		}

		hostname := link.GetHostname()
		u, ok := c.Configuration.router.Load(hostname)
		if !ok {
			c.Logger.Error("Unknown hostname in connection", zap.String("hostname", hostname))
			delegation.Close()
			return
		}

		c.Logger.Info("Incoming connection from gateway",
			zap.String("protocol", link.GetAlpn().String()),
			zap.String("hostname", link.GetHostname()),
			zap.String("remote", link.GetRemote()))

		switch link.GetAlpn() {
		case protocol.Link_HTTP:
			c.getHTTPProxy(ctx, hostname, u).acceptor.Handle(delegation)

		case protocol.Link_TCP:
			c.forwardStream(ctx, hostname, delegation, u)

		default:
			c.Logger.Error("Unknown alpn for forwarding", zap.String("alpn", link.GetAlpn().String()))
			delegation.Close()
		}
	})
	c.attachRPC(ctx, streamRouter)

	go streamRouter.Accept(ctx)
	go c.periodicReconnection(ctx)
	go c.reloadOnSignal(ctx)
	go c.reBootstrap(ctx)
	go c.startLocalServer(ctx)
}

func (c *Client) closeOutdatedProxies(tunnels ...Tunnel) {
	for _, t := range tunnels {
		proxy, loaded := c.proxies.LoadAndDelete(t.Hostname)
		if loaded {
			c.Logger.Info("Shutting down proxy", zap.String("hostname", t.Hostname), zap.String("target", t.Target))
			proxy.acceptor.Close()
			proxy.forwarder.Close()
		}
	}
}

func (c *Client) Close() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	c.rpcAcceptor.Close()
	c.proxies.Range(func(key string, proxy *httpProxy) bool {
		c.Logger.Info("Shutting down proxy", zap.String("hostname", key))
		proxy.acceptor.Close()
		return true
	})
	close(c.closeCh)
	c.closeWg.Wait()
}

func diffTunnels(old, new []Tunnel) []Tunnel {
	diff := make([]Tunnel, 0)
	oldMap := map[string]Tunnel{}
	newMap := map[string]Tunnel{}
	for _, o := range old {
		if o.Hostname == "" {
			continue
		}
		oldMap[o.Hostname] = o
	}
	for _, n := range new {
		if n.Hostname == "" {
			continue
		}
		newMap[n.Hostname] = n
	}
	// if new != old
	for hostname, tunnel := range newMap {
		oldTunnel, ok := oldMap[hostname]
		if ok && (oldTunnel.Target != tunnel.Target || oldTunnel.Insecure != tunnel.Insecure) {
			diff = append(diff, oldTunnel)
		}
	}
	// if old is gone
	for hostname, tunnel := range oldMap {
		if _, ok := newMap[hostname]; !ok {
			diff = append(diff, tunnel)
		}
	}
	return diff
}
