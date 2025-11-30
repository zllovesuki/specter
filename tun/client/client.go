package client

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"sync"
	"time"

	"go.miragespace.co/specter/overlay"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util"
	"go.miragespace.co/specter/util/acceptor"

	"github.com/Yiling-J/theine-go"
	"github.com/zeebo/xxh3"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

var (
	checkInterval     = time.Second * 30
	rttInterval       = transport.RTTMeasureInterval
	certCheckInterval = time.Hour * 24 // Check certificate daily for long-running clients
)

const (
	connectTimeout = time.Second * 5
	rpcTimeout     = time.Second * 5
	renewalWindow  = 30 * 24 * time.Hour // Renew certificates 30 days before expiry
)

type KeylessProxyConfig struct {
	HTTPListner  net.Listener
	HTTPSListner net.Listener
	ALPNMux      *overlay.ALPNMux
}

type ClientConfig struct {
	Logger          *zap.Logger
	Configuration   *Config
	PKIClient       protocol.PKIService
	ServerTransport transport.Transport
	Recorder        rtt.Recorder
	ReloadSignal    <-chan os.Signal
	ServerListener  net.Listener
	KeylessProxy    KeylessProxyConfig
}

type Client struct {
	ClientConfig
	configMu                sync.RWMutex
	closeWg                 sync.WaitGroup
	syncMu                  sync.Mutex
	tunnelClient            rpc.TunnelClient
	parentCtx               context.Context
	rootDomain              *atomic.String
	proxies                 *skipmap.StringMap[*httpProxy]
	connections             *skipmap.StringMap[*protocol.Node]
	rpcAcceptor             *acceptor.HTTP2Acceptor
	keylessCertificateCache *theine.LoadingCache[string, keylessCertificateResult]
	closeCh                 chan struct{}
	closed                  atomic.Bool
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

	keylessCache, err := theine.NewBuilder[string, keylessCertificateResult](cacheTotalCost).
		BuildWithLoader(c.keylesCertificateCacheLoader)
	if err != nil {
		return nil, err
	}
	c.keylessCertificateCache = keylessCache

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

func (c *Client) hash(seed uint64, nodes []*protocol.Node) uint64 {
	var buf [8]byte

	hasher := xxh3.New()

	for _, node := range nodes {
		binary.BigEndian.PutUint64(buf[:], node.GetId())
		hasher.Write(buf[:])
	}
	return hasher.Sum64()
}

func (c *Client) GetConnectedNodes() []*protocol.Node {
	return c.getConnectedNodes()
}

func (c *Client) Start(ctx context.Context) {
	c.Logger.Info("Listening for tunnel traffic")

	c.closeWg.Add(4)

	streamRouter := transport.NewStreamRouter(c.Logger, nil, c.ServerTransport)
	streamRouter.HandleTunnel(protocol.Stream_DIRECT, func(delegation *transport.StreamDelegate) {
		link := &protocol.Link{}
		if err := rpc.BoundedReceive(delegation, link, 1024); err != nil {
			c.Logger.Error("Receiving link information from gateway", zap.Error(err))
			delegation.Close()
			return
		}
		c.handleIncomingDelegation(ctx, link, delegation)
	})
	c.attachRPC(ctx, streamRouter)

	go streamRouter.Accept(ctx)
	go c.periodicReconnection(ctx)
	go c.reloadOnSignal(ctx)
	go c.reBootstrap(ctx)
	go c.certificateMaintainer(ctx)
	go c.startLocalServer(ctx)
	go c.startKeylessProxy()
}

func (c *Client) handleIncomingDelegation(ctx context.Context, link *protocol.Link, delegation net.Conn) error {
	hostname := link.GetHostname()
	u, ok := c.Configuration.router.Load(hostname)
	if !ok {
		c.Logger.Error("Unknown hostname in connection", zap.String("hostname", hostname))
		delegation.Close()
		return tun.ErrDestinationNotFound
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
		return tun.ErrDestinationNotFound
	}

	return nil
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
