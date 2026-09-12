package client

import (
	"context"
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
	*forwarder
	configMu                sync.RWMutex
	closeWg                 sync.WaitGroup
	syncMu                  sync.Mutex
	syncStateMu             sync.RWMutex
	publication             map[string]publicationState
	hostnameNextCandidate   uint64
	lastSync                SyncResult
	nextSync                time.Time
	syncBackoff             time.Duration
	tunnelClient            rpc.TunnelClient
	parentCtx               context.Context
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
		forwarder:    newForwarder(cfg.Logger),
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
		c.Logger = c.Logger.With(zap.Uint64("id", c.ServerTransport.Identity().GetId()))
		c.forwarder.logger = c.Logger
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
		if result := c.SyncConfigTunnels(ctx); result.Error != "" {
			c.Logger.Warn("Initial tunnel synchronization is pending retry", zap.String("error", result.Error))
		}
	}
	return nil
}

func (c *Client) GetConnectedNodes() []*protocol.Node {
	return c.getConnectedNodes()
}

func (c *Client) Start(ctx context.Context) {
	c.Logger.Info("Listening for tunnel traffic")

	c.closeWg.Add(3)

	streamRouter := transport.NewStreamRouter(c.Logger, nil, c.ServerTransport)
	streamRouter.HandleTunnel(protocol.Stream_DIRECT, func(delegation *transport.StreamDelegate) {
		link, ok := receiveLink(c.Logger, delegation)
		if !ok {
			return
		}
		c.handleIncomingDelegation(ctx, link, delegation)
	})
	c.attachRPC(ctx, streamRouter)

	go streamRouter.Accept(ctx)
	go c.periodicReconnection(ctx)
	go c.reloadOnSignal(ctx)
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

	return c.handleLink(ctx, link, delegation, u)
}

func (c *Client) Close() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}
	c.rpcAcceptor.Close()
	c.closeAll()
	close(c.closeCh)
	c.closeWg.Wait()
}
