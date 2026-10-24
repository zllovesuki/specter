package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"go.miragespace.co/specter/acme"
	chordImpl "go.miragespace.co/specter/chord"
	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/gateway"
	"go.miragespace.co/specter/internal/listen"
	"go.miragespace.co/specter/overlay"
	"go.miragespace.co/specter/pki"
	"go.miragespace.co/specter/rtt"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/cipher"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/transport/q"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/timing"
	"go.miragespace.co/specter/tun/server"
	"go.miragespace.co/specter/util/migrator"
	"go.miragespace.co/specter/util/reuse"
	"go.miragespace.co/specter/waf"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/alecthomas/units"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/pires/go-proxyproto"
	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Options supplies the physical server's startup configuration.
// Nil optional fields preserve the command's distinction between an absent flag
// and an explicitly supplied empty value or zero port.
type Options struct {
	Logger           *zap.Logger
	Version          string
	DataDir          string
	KVProvider       string
	ListenAddresses  []string
	ListenTCP        []string
	ListenUDP        []string
	AdvertiseAddress *string
	RPCListen        *string
	HTTPPort         *int
	ProxyProtocol    bool
	CertDirectory    *string
	CertEnvironment  *CertificateEnvironment
	VirtualNodes     int
	JoinAddress      *string
	SentryDSN        *string
	Apex             []string
	ACMEEnabled      bool
	ACMECA           string
	ACMEEmail        string
	ACMEZone         string
	ProxyBuffer      string
	TransportBuffer  string
	AdminUser        string
	AdminPass        string
}

func modifyToSentryLogger(logger *zap.Logger, client *sentry.Client) *zap.Logger {
	cfg := zapsentry.Configuration{
		Level:             zapcore.WarnLevel,
		EnableBreadcrumbs: true,
		BreadcrumbLevel:   zapcore.InfoLevel,
	}
	core, err := zapsentry.NewCore(cfg, zapsentry.NewSentryClientFromClient(client))

	if err != nil {
		logger.Warn("failed to init zap", zap.Error(err))
	}

	logger = zapsentry.AttachCoreToLogger(core, logger)

	return logger
}

// Run starts the physical server and shuts it down on a process signal or context cancellation.
func Run(ctx context.Context, options Options) error {
	logger := options.Logger
	rootDomains := options.Apex
	managedDomains := rootDomains

	dataDir, err := filepath.Abs(options.DataDir)
	if err != nil {
		return fmt.Errorf("resolving data directory: %w", err)
	}
	kvOption := options.KVProvider
	if err := preflightKVProvider(kvOption, dataDir); err != nil {
		return fmt.Errorf("storage preflight: %w", err)
	}
	logger.Info("Storage configuration", zap.String("provider", kvOption), zap.String("data_dir", dataDir))
	configStore, err := config.Open(filepath.Join(dataDir, "config"))
	if err != nil {
		return fmt.Errorf("opening shared configuration: %w", err)
	}
	defer configStore.Close()

	var gwOptions gateway.Options
	proxyBuffer, err := units.ParseStrictBytes(options.ProxyBuffer)
	if err != nil {
		return fmt.Errorf("error parsing proxy buffer size: %w", err)
	}
	gwOptions.ProxyBufferSize = int(proxyBuffer)
	transportBuffer, err := units.ParseStrictBytes(options.TransportBuffer)
	if err != nil {
		return fmt.Errorf("error parsing transport buffer size: %w", err)
	}
	gwOptions.TransportBufferSize = int(transportBuffer)

	if options.SentryDSN != nil {
		client, err := sentry.NewClient(sentry.ClientOptions{
			Dsn:     *options.SentryDSN,
			Release: options.Version,
		})
		if err != nil {
			return fmt.Errorf("initializing sentry client: %w", err)
		}
		defer client.Flush(time.Second * 2)

		logger = modifyToSentryLogger(logger, client)
		defer logger.Sync()
	}

	listenBase := options.ListenAddresses
	tcpAddrs, err := listen.ParseAddresses("tcp",
		listenBase,
		options.ListenTCP,
	)
	if err != nil {
		return fmt.Errorf("error parsing tcp listen address: %w", err)
	}

	udpAddrs, err := listen.ParseAddresses("udp",
		listenBase,
		options.ListenUDP,
	)
	if err != nil {
		return fmt.Errorf("error parsing udp listen address: %w", err)
	}

	addrStrings := func(addrs []listen.Address) []string {
		out := make([]string, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, a.Address)
		}
		return out
	}

	logger.Info("listener configuration",
		zap.Strings("tcp", addrStrings(tcpAddrs)),
		zap.Strings("udp", addrStrings(udpAddrs)),
	)

	if len(listenBase) == 0 {
		return fmt.Errorf("at least one listen-addr must be provided")
	}

	advertise := listenBase[0]

	if options.AdvertiseAddress != nil {
		advertise = *options.AdvertiseAddress
	}
	_, advertisePortStr, err := net.SplitHostPort(advertise)
	if err != nil {
		return fmt.Errorf("error parsing advertise address: %w", err)
	}
	advertisePort, err := strconv.ParseInt(advertisePortStr, 10, 32)
	if err != nil {
		return fmt.Errorf("error parsing advertise port: %w", err)
	}

	var bundle *certBundle
	if options.CertDirectory != nil {
		bundle, err = certLoaderFilesystem(*options.CertDirectory)
		if err != nil {
			return fmt.Errorf("error loading certificates from directory: %w", err)
		}
	} else if options.CertEnvironment != nil {
		bundle, err = certLoaderEnv(options.CertEnvironment)
		if err != nil {
			return fmt.Errorf("error loading certificates from environment variable: %w", err)
		}
	}

	listenCfg := &net.ListenConfig{
		Control: reuse.Control,
	}

	var (
		rpcListener net.Listener
	)
	if options.RPCListen != nil {
		parsedRpc, err := url.Parse(*options.RPCListen)
		if err != nil {
			return fmt.Errorf("error parsing rpc listen address: %w", err)
		}
		switch parsedRpc.Scheme {
		case "unix":
			rpcListener, err = listenCfg.Listen(ctx, "unix", parsedRpc.Path)
		case "tcp":
			rpcListener, err = listenCfg.Listen(ctx, "tcp", parsedRpc.Host)
		default:
			return fmt.Errorf("unknown scheme for rpc listen address: %s", parsedRpc.Scheme)
		}
		if err != nil {
			return fmt.Errorf("error setting up rpc listener: %w", err)
		}
		defer rpcListener.Close()
	}

	tcpListeners := make([]net.Listener, 0, len(tcpAddrs))
	for _, addr := range tcpAddrs {
		l, err := listenCfg.Listen(ctx, addr.Network, addr.Address)
		if err != nil {
			return fmt.Errorf("error setting up gateway tcp listener on %s: %w", addr.Address, err)
		}
		if options.ProxyProtocol {
			l = &proxyproto.Listener{
				Listener:          l,
				ReadHeaderTimeout: time.Second * 3,
				Policy: func(upstream net.Addr) (proxyproto.Policy, error) {
					return proxyproto.REQUIRE, nil
				},
			}
		}
		tcpListeners = append(tcpListeners, l)
	}
	if len(tcpListeners) == 0 {
		return fmt.Errorf("no tcp listeners configured")
	}

	// TODO: implement SNI proxy so specter can share port with another webserver
	tcpListener := tcpListeners[0]
	if len(tcpListeners) > 1 {
		tcpListener = newMultiListener(tcpListeners)
	}
	defer tcpListener.Close()

	udpBindings := make([]udpBinding, 0, len(udpAddrs))
	for _, addr := range udpAddrs {
		pconn, err := listenCfg.ListenPacket(ctx, addr.Network, addr.Address)
		if err != nil {
			return fmt.Errorf("error setting up gateway udp listener on %s: %w", addr.Address, err)
		}
		tr := &quic.Transport{Conn: pconn}
		udpBindings = append(udpBindings, udpBinding{
			listen:     addr,
			packetConn: pconn,
			transport:  tr,
		})
	}
	if len(udpBindings) == 0 {
		return fmt.Errorf("no udp listeners configured")
	}
	for i := range udpBindings {
		defer udpBindings[i].transport.Close()
		defer udpBindings[i].packetConn.Close()
	}

	var httpListener net.Listener
	if advertisePort == 443 || options.HTTPPort != nil {
		httpPort := 80
		if options.HTTPPort != nil {
			httpPort = *options.HTTPPort
		}
		httpListeners := make([]net.Listener, 0, len(tcpAddrs))
		seenHost := make(map[string]struct{}, len(tcpAddrs))
		for _, addr := range tcpAddrs {
			if _, ok := seenHost[addr.Host]; ok {
				continue
			}
			seenHost[addr.Host] = struct{}{}
			httpAddr := net.JoinHostPort(addr.Host, strconv.Itoa(httpPort))
			l, err := listenCfg.Listen(ctx, listen.NetworkForVersion("tcp", addr.Version), httpAddr)
			if err != nil {
				return fmt.Errorf("error setting up http listener on %s: %w", httpAddr, err)
			}
			if options.ProxyProtocol {
				l = &proxyproto.Listener{
					Listener:          l,
					ReadHeaderTimeout: time.Second * 3,
					Policy: func(upstream net.Addr) (proxyproto.Policy, error) {
						return proxyproto.REQUIRE, nil
					},
				}
			}
			httpListeners = append(httpListeners, l)
		}

		if len(httpListeners) > 0 {
			httpListener = httpListeners[0]
			if len(httpListeners) > 1 {
				httpListener = newMultiListener(httpListeners)
			}
			defer httpListener.Close()
		}
	}

	alpnMuxes := make([]*overlay.ALPNMux, 0, len(udpBindings))
	for _, binding := range udpBindings {
		mux, err := overlay.NewMux(binding.transport)
		if err != nil {
			return fmt.Errorf("error setting up quic alpn muxer for %s: %w", binding.listen.Address, err)
		}
		alpnMuxes = append(alpnMuxes, mux)
		defer mux.Close()
	}

	chordName := fmt.Sprintf("chord://%s", advertise)
	tunnelName := fmt.Sprintf("tunnel://%s", advertise)

	logger.Info("Using advertise addresses as destinations", zap.String("chord", chordName), zap.String("tunnel", tunnelName))

	chordTLS := cipher.GetPeerTLSConfig(bundle.ca, bundle.node, []string{
		tun.ALPN(protocol.Link_SPECTER_CHORD),
	})

	// handles specter-chord/1
	chordListeners := make([]q.Listener, 0, len(alpnMuxes))
	for _, mux := range alpnMuxes {
		chordListeners = append(chordListeners, mux.With(chordTLS, tun.ALPN(protocol.Link_SPECTER_CHORD)))
	}
	chordListener := newMultiQuicListener(ctx, chordListeners)
	defer chordListener.Close()

	chordRTT := rtt.NewInstrumentation(20)
	dialer := newMultiDialer(udpBindings)
	chordTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:           logger.With(zapsentry.NewScope()).With(zap.String("component", "chordTransport")),
		VirtualTransport: true,
		ClientTLS:        chordTLS,
		RTTRecorder:      chordRTT,
		QuicTransport:    dialer,
		Endpoint: &protocol.Node{
			Address: advertise,
		},
	})
	defer chordTransport.Stop()

	// TODO: measure rtt to client to build routing table with cost
	tunnelTransport := overlay.NewQUIC(overlay.TransportConfig{
		UseCertificateIdentity: true,
		Logger:                 logger.With(zapsentry.NewScope()).With(zap.String("component", "tunnelTransport")),
		Endpoint: &protocol.Node{
			Address: advertise,
		},
		QuicTransport: dialer,
	})
	defer tunnelTransport.Stop()

	var existingNode chord.VNode
	chordClient := rpc.DynamicChordClient(ctx, chordTransport)
	if options.JoinAddress != nil {
		existingNode, err = chordImpl.NewRemoteNode(ctx, logger, chordClient, &protocol.Node{
			Unknown: true,
			Address: *options.JoinAddress,
		})
		if err != nil {
			return fmt.Errorf("error connecting existing chord node: %w", err)
		}
	}

	streamRouter := transport.NewStreamRouter(logger.With(zapsentry.NewScope()).With(zap.String("component", "router")), chordTransport, tunnelTransport)
	virtualNodes := make([]*chordImpl.LocalNode, 0)

	k := options.VirtualNodes
	cacheDir := filepath.Join(dataDir, "cache")
	for i := range k {
		nodeIdentity := &protocol.Node{
			Id:      chord.Hash(fmt.Appendf(nil, "%s/%d", chordName, i)),
			Address: advertise,
		}
		kvProvider, stopFn, err := getKVProvider(
			logger.With(zapsentry.NewScope()).With(zap.String("component", "kv"), zap.Object("node", nodeIdentity)),
			kvOption,
			filepath.Join(dataDir, fmt.Sprintf("%d", i)),
			cacheDir,
		)
		if err != nil {
			return fmt.Errorf("initializing kv storage: %w", err)
		}
		defer stopFn()

		virtualNode := chordImpl.NewLocalNode(chordImpl.NodeConfig{
			Identity:                 nodeIdentity,
			BaseLogger:               logger,
			ChordClient:              chordClient,
			KVProvider:               kvProvider,
			StabilizeInterval:        timing.ChordStabilizeInterval,
			FixFingerInterval:        timing.ChordFixFingerInterval,
			PredecessorCheckInterval: timing.ChordPredecessorCheckInterval,
			NodesRTT:                 chordRTT,
		})

		virtualNode.AttachRouter(ctx, streamRouter)
		virtualNodes = append(virtualNodes, virtualNode)
	}

	rootNode := virtualNodes[0]
	rootNode.AttachRoot(ctx, streamRouter)
	var configJoin *protocol.Node
	if existingNode != nil {
		configJoin = existingNode.Identity()
	}
	configCtx, stopConfig := context.WithCancel(ctx)
	configSync := config.NewSyncer(configStore, chordTransport, func() []*protocol.Node {
		return sharedConfigPeers(chordTransport, virtualNodes, configJoin)
	}, logger.With(zap.String("component", "shared_config")))
	configSync.Bind(configCtx, streamRouter)
	configDone := make(chan struct{})
	go func() {
		defer close(configDone)
		configSync.Run(configCtx)
	}()
	defer func() {
		stopConfig()
		<-configDone
	}()
	if rpcListener != nil {
		logger.Info("Exposing RPC externally", zap.String("listen", *options.RPCListen))
		stopRPC := startLocalRPC(ctx, rpcListener, rootNode.RPCHandler(ctx), configSync, logger)
		defer stopRPC()
	}

	go chordTransport.AcceptWithListener(ctx, chordListener)
	go streamRouter.Accept(ctx)
	for _, mux := range alpnMuxes {
		go mux.Accept(ctx)
	}

	if options.JoinAddress == nil {
		if err := rootNode.Create(); err != nil {
			return fmt.Errorf("error bootstrapping chord ring: %w", err)
		}
	} else {
		if err := rootNode.Join(existingNode); err != nil {
			return fmt.Errorf("error joining root node to existing chord ring: %w", err)
		}
	}
	defer rootNode.Leave()

	for i := 1; i < k; i++ {
		p, err := chordImpl.NewRemoteNode(ctx, logger, chordClient, rootNode.Identity())
		if err != nil {
			return fmt.Errorf("error connecting to root node: %w", err)
		}
		if err := virtualNodes[i].Join(p); err != nil {
			return fmt.Errorf("error joining virtual node to root node: %w", err)
		}
		defer virtualNodes[i].Leave()
	}

	certProvider, err := configCertProvider(options, logger.With(zapsentry.NewScope()), chord.WrapRetryKV(rootNode, timing.ChordStabilizeInterval/2, 5))
	if err != nil {
		return fmt.Errorf("failed to configure cert provider: %w", err)
	}

	if err := certProvider.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize cert provider: %w", err)
	}

	gwTLSConf := cipher.GetGatewayTLSConfig(certProvider.GetCertificate, []string{
		tun.ALPN(protocol.Link_HTTP2),
		tun.ALPN(protocol.Link_HTTP),
		tun.ALPN(protocol.Link_TCP),
		tun.ALPN(protocol.Link_UNKNOWN),
	})

	gwH2Listener := tls.NewListener(tcpListener, gwTLSConf)
	defer gwH2Listener.Close()

	// handles h3, h3-29, and specter-tcp/1
	gwH3Listeners := make([]q.Listener, 0, len(alpnMuxes))
	for _, mux := range alpnMuxes {
		gwH3Listeners = append(gwH3Listeners, mux.With(gwTLSConf, append([]string{tun.ALPN(protocol.Link_TCP)}, cipher.H3Protos...)...))
	}
	gwH3Listener := newMultiQuicListener(ctx, gwH3Listeners)
	defer gwH3Listener.Close()

	// handles specter-client/1
	clientTLSConf := cipher.GetClientTLSConfig(bundle.clientCa, certProvider.GetCertificate, []string{tun.ALPN(protocol.Link_SPECTER_CLIENT)})
	clientListeners := make([]q.Listener, 0, len(alpnMuxes))
	for _, mux := range alpnMuxes {
		clientListeners = append(clientListeners, mux.With(clientTLSConf, tun.ALPN(protocol.Link_SPECTER_CLIENT)))
	}
	clientListener := newMultiQuicListener(ctx, clientListeners)
	defer clientListener.Close()

	tunnelIdentity := &protocol.Node{
		Id:      chord.Hash([]byte(tunnelName)),
		Address: advertise,
	}
	tunServer := server.New(server.Config{
		Logger:          logger.With(zapsentry.NewScope()).With(zap.String("component", "tunnelServer"), zap.Uint64("node", tunnelIdentity.GetId())),
		ParentContext:   ctx,
		Chord:           chord.WrapRetryKV(rootNode, timing.ChordStabilizeInterval/2, 5),
		TunnelTransport: tunnelTransport,
		ChordTransport:  chordTransport,
		Resolver:        net.DefaultResolver,
		CertProvider:    certProvider,
		Apex:            rootDomains[0],
		Acme:            options.ACMEZone,
	})
	defer tunServer.Stop()

	tunServer.AttachRouter(ctx, streamRouter)
	tunServer.MustRegister(ctx)

	go tunnelTransport.AcceptWithListener(ctx, clientListener)
	wafManager, err := waf.NewManager(configStore, filepath.Join(dataDir, "waf"), logger.With(zap.String("component", "waf")), tunServer.WAFPolicy)
	if err != nil {
		return fmt.Errorf("initializing web protection: %w", err)
	}
	wafCtx, stopWAF := context.WithCancel(ctx)
	wafDone := make(chan struct{})
	go func() {
		defer close(wafDone)
		wafManager.Run(wafCtx)
	}()
	defer func() {
		stopWAF()
		<-wafDone
		_ = wafManager.Close()
	}()

	configuration := chi.NewRouter()
	configuration.Handle("/dns", acme.DNSConfigHandler(configStore, rootNode.Identity(), options.ACMEZone, logger, configSync.Wake))
	configuration.Handle("/waf", waf.SettingsHandler(configStore, rootNode.Identity(), wafManager.Status, func() {
		configSync.Wake()
		wafManager.Wake()
	}, logger))

	var acmeHandler http.Handler
	if mgr, ok := certProvider.(*acme.Manager); ok {
		acmeHandler = acme.AcmeManagerHandler(mgr)
	}
	gw := gateway.New(gateway.GatewayConfig{
		PKIServer: &pki.Server{
			Logger:   logger.With(zapsentry.NewScope()).With(zap.String("component", "pki")),
			ClientCA: bundle.clientCaCert,
		},
		Handlers: gateway.InternalHandlers{
			Configuration: configuration,
			Acme:          acmeHandler,
			Chord:         chordImpl.ChordStatsHandler(rootNode, virtualNodes),
			Overview:      chordImpl.OverviewHandler(rootNode, virtualNodes, kvOption),
			TunnelServer:  server.TunnelServerHandler(tunServer),
			Migrator:      migrator.ConfigMigratorHandler(logger.With(zapsentry.NewScope()).With(zap.String("component", "migrator")), bundle.clientCaCert),
		},
		Logger:                logger.With(zapsentry.NewScope()).With(zap.String("component", "gateway")),
		TunnelServer:          tunServer,
		HTTPListener:          httpListener,
		H2Listener:            gwH2Listener,
		H3Listener:            gwH3Listener,
		RootDomains:           managedDomains,
		GatewayPort:           int(advertisePort),
		Options:               gwOptions,
		AdminUser:             options.AdminUser,
		AdminPass:             options.AdminPass,
		HandshakeHintFunc:     tunServer.RoutesPreload,
		RouteCheckFunc:        tunServer.CheckRoute,
		ApplicationMiddleware: wafManager.Wrap,
	})
	defer gw.Close()

	gw.AttachRouter(ctx, streamRouter)
	gw.MustStart(ctx)

	certProvider.OnHandshake(gw.HandshakeEarlyHint)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)

	select {
	case sig := <-sigs:
		logger.Info("received signal to stop", zap.String("signal", sig.String()))
	case <-ctx.Done():
		logger.Info("context done", zap.Error(ctx.Err()))
	}

	return nil
}
