package server

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"kon.nect.sh/specter/acme"
	chordImpl "kon.nect.sh/specter/chord"
	"kon.nect.sh/specter/gateway"
	"kon.nect.sh/specter/kv/aof"
	"kon.nect.sh/specter/overlay"
	"kon.nect.sh/specter/pki"
	"kon.nect.sh/specter/rtt"
	acmeSpec "kon.nect.sh/specter/spec/acme"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/cipher"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/server"
	"kon.nect.sh/specter/util"
	"kon.nect.sh/specter/util/migrator"
	"kon.nect.sh/specter/util/reuse"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/getsentry/sentry-go"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Generate() *cli.Command {
	ip := util.GetOutboundIP()
	return &cli.Command{
		Name:  "server",
		Usage: "start an specter server on the edge",
		Description: `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Duis fringilla suscipit tincidunt. Aenean ut sem ipsum.

	Specter server provides an internal endpoint on /_internal under apex domain. To enable internal endpoint, provide username and password 
	under environment variables INTERNAL_USER and INTERNAL_PASS. Absent of them will disable the internal endpoint entirely.

	Warning: do not use certificates issued by public CA for inter-node certificates, otherwise anyone can join your specter network`,
		ArgsUsage: " ",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "advertise-addr",
				Aliases:     []string{"advertise"},
				DefaultText: "same as listen-addr",
				Value:       fmt.Sprintf("%s:443", ip.String()),
				Usage: `Address and port to advertise to specter servers and clients to connect to.
			Note that specter will use advertised address to derive its Identity hash.`,
				Category: "Network Options",
			},
			&cli.StringFlag{
				Name:    "listen-addr",
				Aliases: []string{"listen"},
				Value:   fmt.Sprintf("%s:443", ip.String()),
				Usage: `Address and port to listen for specter server, specter client and gateway connections. This port will serve both TCP and UDP (unless overridden).
			Note that if specter is listening on port 443, it will also listen on port 80 to handle http connect proxy, and redirect other http requests to https`,
				Category: "Network Options",
			},
			&cli.StringFlag{
				Name:        "listen-tcp",
				DefaultText: "same as listen-addr",
				Usage:       "Override the listen address and port for TCP",
				Category:    "Network Options",
			},
			&cli.StringFlag{
				Name:        "listen-udp",
				DefaultText: "same as listen-addr",
				Usage:       "Override the listen address and port for UDP. Required if environment needs a specific address, such as on fly.io",
				Category:    "Network Options",
			},

			&cli.StringFlag{
				Name:  "listen-rpc",
				Value: "tcp://127.0.0.1:11180",
				Usage: `Expose chord's RPC for VNode and KV to an external program. This is required to use with specter's acme dns.
			NOTE: The listener is exposed without any authentication or authorization. You should only expose it to localhost or unix socket`,
				Category: "Server Options",
			},
			&cli.PathFlag{
				Name:     "data-dir",
				Aliases:  []string{"data"},
				Usage:    "Path to directory that will be used for persisting non-volatile KV data",
				Required: true,
				Category: "Server Options",
			},
			&cli.PathFlag{
				Name:     "cert-dir",
				Aliases:  []string{"cert"},
				Usage:    `Path to directory containing ca.crt, client-ca.crt, client-ca.key, node.crt, and node.key for mutual TLS between specter server nodes`,
				Category: "Server Options",
			},
			&cli.BoolFlag{
				Name: "cert-env",
				Usage: `Load ca.crt (CERT_CA), client-ca.crt (CERT_CLIENT_CA), client-ca.key (CERT_CLIENT_CA_KEY), node.crt (CERT_NODE), and node.key (CERT_NODE_KEY) from environment variables encoded as base64.
			This can be set instead of loading from cert-dir. Required if environment prefers loading secrets from ENV, such as on fly.io`,
				Category: "Server Options",
			},
			&cli.StringFlag{
				Name:        "sentry",
				DefaultText: "https://public@sentry.example.com/1",
				Usage:       "Sentry DSN for error monitoring. Alternatively, you can set the DSN via the environment variable SENTRY_DSN",
				EnvVars:     []string{"SENTRY_DSN"},
				Category:    "Server Options",
			},

			&cli.IntFlag{
				Name:     "virtual",
				Usage:    "Number of virtual nodes to be started as part of the chord ring",
				Value:    5,
				Category: "Chord Options",
			},
			&cli.StringFlag{
				Name: "join",
				Usage: `A known specter server's advertise address.
			Absent of this flag will bootstrap a new cluster with current node as the seed node`,
				Category: "Chord Options",
			},

			&cli.StringFlag{
				Name:        "acme",
				DefaultText: "acme://{ACME_EMAIL}:@acmehostedzone.com",
				EnvVars:     []string{"ACME_URI"},
				Usage: `To enable acme, provide an email for the issuer, and the delegated zone for hosting challenges.
			Absent of this flag will serve self-signed certificate.
			Alternatively, you can set the URI via the environment variable ACME_URI.`,
				Category: "Gateway Options",
			},
			&cli.IntFlag{
				Name:  "listen-http",
				Value: 80,
				Usage: `Override the listening port of the http handler, which handles http connect proxy, and redirects other http requests to https.
			Note by default the http handler will not be started unless the node is advertising on port 443. Using this option will force the http handler to start.`,
				Category: "Gateway Options",
			},
			&cli.StringFlag{
				Name:     "apex",
				Usage:    "Canonical domain to be used as tunnel root domain. Tunnels will be given names under *.`APEX`",
				Required: true,
				Category: "Gateway Options",
			},
			&cli.StringSliceFlag{
				Name:     "extra-apex",
				Usage:    "Additional canonical domains, useful for redundant tunnel hostnames. Tunnels will also be available under *.`EXTRA_APEX`",
				Category: "Gateway Options",
			},

			&cli.BoolFlag{
				Name:     "print-acme",
				Value:    false,
				Usage:    "Print acme setup instructions for apex domains based on current configuration.",
				Category: "Miscellaneous",
			},

			&cli.StringFlag{
				Name:    "acme_ca",
				Hidden:  true,
				Value:   cipher.CertCA,
				EnvVars: []string{"ACME_CA"},
			},

			// used for acme setup internally
			&cli.StringFlag{
				Name:   "acme_email",
				Hidden: true,
			},
			&cli.StringFlag{
				Name:   "acme_zone",
				Hidden: true,
			},
			&cli.StringFlag{
				Name:    "auth_user",
				Hidden:  true,
				EnvVars: []string{"INTERNAL_USER"},
			},
			&cli.StringFlag{
				Name:    "auth_pass",
				Hidden:  true,
				EnvVars: []string{"INTERNAL_PASS"},
			},
			&cli.StringFlag{
				Name:    "env_ca",
				Hidden:  true,
				EnvVars: []string{"CERT_CA"},
			},
			&cli.StringFlag{
				Name:    "env_node",
				Hidden:  true,
				EnvVars: []string{"CERT_NODE"},
			},
			&cli.StringFlag{
				Name:    "env_node_key",
				Hidden:  true,
				EnvVars: []string{"CERT_NODE_KEY"},
			},
			&cli.StringFlag{
				Name:    "env_client_ca",
				Hidden:  true,
				EnvVars: []string{"CERT_CLIENT_CA"},
			},
			&cli.StringFlag{
				Name:    "env_client_ca_key",
				Hidden:  true,
				EnvVars: []string{"CERT_CLIENT_CA_KEY"},
			},
		},
		Before: func(ctx *cli.Context) error {
			if !ctx.IsSet("cert-dir") && !ctx.IsSet("cert-env") {
				return fmt.Errorf("no certificate loader is specified")
			}
			if ctx.Int("virtual") < 1 {
				return fmt.Errorf("minimum of 1 virtual node is required")
			}
			if ctx.IsSet("acme") {
				email, zone, err := acmeSpec.ParseAcmeURI(ctx.String("acme"))
				if err != nil {
					return err
				}
				ctx.Set("acme_email", email)
				ctx.Set("acme_zone", zone)
			}
			return nil
		},
		Action: cmdServer,
	}
}

type certBundle struct {
	ca           *x509.CertPool
	clientCa     *x509.CertPool
	clientCaCert tls.Certificate
	node         tls.Certificate
}

func certLoaderFilesystem(dir string) (*certBundle, error) {
	files := []string{"ca.crt", "client-ca.crt", "client-ca.key", "node.crt", "node.key"}
	for i, name := range files {
		files[i] = filepath.Join(dir, name)
	}
	caCert, err := os.ReadFile(files[0])
	if err != nil {
		return nil, fmt.Errorf("reading ca cert from file: %w", err)
	}
	clientCaCert, err := os.ReadFile(files[1])
	if err != nil {
		return nil, fmt.Errorf("reading client ca cert from file: %w", err)
	}
	clientCaKey, err := os.ReadFile(files[2])
	if err != nil {
		return nil, fmt.Errorf("reading client ca key from file: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("unable to use provided ca bundle")
	}
	clientCaCertPool := x509.NewCertPool()
	if ok := clientCaCertPool.AppendCertsFromPEM(clientCaCert); !ok {
		return nil, fmt.Errorf("unable to use provided client ca bundle")
	}
	node, err := tls.LoadX509KeyPair(files[3], files[4])
	if err != nil {
		return nil, fmt.Errorf("creating node cert/key from files: %w", err)
	}
	clientCa, err := tls.X509KeyPair(clientCaCert, clientCaKey)
	if err != nil {
		return nil, fmt.Errorf("creating client ca cert/key from files: %w", err)
	}
	return &certBundle{
		ca:           caCertPool,
		clientCa:     clientCaCertPool,
		clientCaCert: clientCa,
		node:         node,
	}, nil
}

func certLoaderEnv(ctx *cli.Context) (*certBundle, error) {
	caCert, err := base64.StdEncoding.DecodeString(ctx.String("env_ca"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_CA: %w", err)
	}
	nodeCert, err := base64.StdEncoding.DecodeString(ctx.String("env_node"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE: %w", err)
	}
	nodeKey, err := base64.StdEncoding.DecodeString(ctx.String("env_node_key"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE_KEY: %w", err)
	}
	clientCaCert, err := base64.StdEncoding.DecodeString(ctx.String("env_client_ca"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE: %w", err)
	}
	clientCaKey, err := base64.StdEncoding.DecodeString(ctx.String("env_client_ca_key"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE_KEY: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("unable to use provided ca bundle")
	}
	clientCaCertPool := x509.NewCertPool()
	if ok := clientCaCertPool.AppendCertsFromPEM(clientCaCert); !ok {
		return nil, fmt.Errorf("unable to use provided client ca bundle")
	}
	node, err := tls.X509KeyPair(nodeCert, nodeKey)
	if err != nil {
		return nil, fmt.Errorf("creating node cert/key from env: %w", err)
	}
	clientCa, err := tls.X509KeyPair(clientCaCert, clientCaKey)
	if err != nil {
		return nil, fmt.Errorf("creating client ca cert/key from env: %w", err)
	}
	return &certBundle{
		ca:           caCertPool,
		clientCa:     clientCaCertPool,
		clientCaCert: clientCa,
		node:         node,
	}, nil
}

func configCertProvider(ctx *cli.Context, logger *zap.Logger, kv chord.KV) (cipher.CertProvider, error) {
	rootDomain := ctx.String("apex")
	extraRootDomains := ctx.StringSlice("extra-apex")
	managedDomains := append([]string{rootDomain}, extraRootDomains...)

	if ctx.IsSet("acme") {
		acmeSolver := &acme.ChordSolver{
			KV:             kv,
			ManagedDomains: managedDomains,
		}
		manager, err := acme.NewManager(ctx.Context, acme.ManagerConfig{
			Logger:         logger,
			KV:             kv,
			DNSSolver:      acmeSolver,
			ManagedDomains: managedDomains,
			CA:             ctx.String("acme_ca"),
			Email:          ctx.String("acme_email"),
		})
		if err != nil {
			return nil, err
		}
		logger.Info("Using acme as cert provider", zap.String("email", ctx.String("acme_email")), zap.String("zone", ctx.String("acme_zone")))
		return manager, nil
	} else {
		logger.Info("Using self-signed as cert provider")
		self := &SelfSignedProvider{
			RootDomain: rootDomain,
		}
		return self, nil
	}
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

func cmdServer(ctx *cli.Context) error {
	logger, ok := ctx.App.Metadata["logger"].(*zap.Logger)
	if !ok || logger == nil {
		return fmt.Errorf("unable to obtain logger from app context")
	}

	rootDomain := ctx.String("apex")
	extraRootDomains := ctx.StringSlice("extra-apex")
	managedDomains := append([]string{rootDomain}, extraRootDomains...)

	if ctx.Bool("print-acme") {
		if !ctx.IsSet("acme") {
			return fmt.Errorf("acme is not configured")
		}
		for _, d := range managedDomains {
			hostname, err := acmeSpec.Normalize(d)
			if err != nil {
				return fmt.Errorf("error normalizing domain for %s: %w", d, err)
			}
			name, content := acmeSpec.GenerateRecord(hostname, ctx.String("acme_zone"), nil)
			logger.Info("ACME DNS Record", zap.String("name", name), zap.String("content", content), zap.String("type", "CNAME"))
		}
		return nil
	}

	if ctx.IsSet("sentry") {
		client, err := sentry.NewClient(sentry.ClientOptions{
			Dsn:     ctx.String("sentry"),
			Release: ctx.App.Version,
		})
		if err != nil {
			return fmt.Errorf("initializing sentry client: %w", err)
		}
		defer client.Flush(time.Second * 2)

		logger = modifyToSentryLogger(logger, client)
		defer logger.Sync()
	}

	var (
		listenTcp = ctx.String("listen-addr")
		listenUdp = ctx.String("listen-addr")
		advertise = ctx.String("listen-addr")
	)

	if ctx.IsSet("listen-tcp") {
		listenTcp = ctx.String("listen-tcp")
	}
	tcpHost, _, err := net.SplitHostPort(listenTcp)
	if err != nil {
		return fmt.Errorf("error parsing tcp listen address: %w", err)
	}

	if ctx.IsSet("listen-udp") {
		listenUdp = ctx.String("listen-udp")
	}
	_, _, err = net.SplitHostPort(listenUdp)
	if err != nil {
		return fmt.Errorf("error parsing udp listen address: %w", err)
	}

	if ctx.IsSet("advertise-addr") {
		advertise = ctx.String("advertise-addr")
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
	if ctx.IsSet("cert-dir") {
		bundle, err = certLoaderFilesystem(ctx.Path("cert-dir"))
		if err != nil {
			return fmt.Errorf("error loading certificates from directory: %w", err)
		}
	} else if ctx.IsSet("cert-env") {
		bundle, err = certLoaderEnv(ctx)
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
	if ctx.IsSet("listen-rpc") {
		parsedRpc, err := url.Parse(ctx.String("listen-rpc"))
		if err != nil {
			return fmt.Errorf("error parsing rpc listen address: %w", err)
		}
		switch parsedRpc.Scheme {
		case "unix":
			rpcListener, err = listenCfg.Listen(ctx.Context, "unix", parsedRpc.Path)
		case "tcp":
			rpcListener, err = listenCfg.Listen(ctx.Context, "tcp", parsedRpc.Host)
		default:
			return fmt.Errorf("unknown scheme for rpc listen address: %s", parsedRpc.Scheme)
		}
		if err != nil {
			return fmt.Errorf("error setting up rpc listener: %w", err)
		}
		defer rpcListener.Close()
	}

	// TODO: implement SNI proxy so specter can share port with another webserver
	tcpListener, err := listenCfg.Listen(ctx.Context, "tcp", listenTcp)
	if err != nil {
		return fmt.Errorf("error setting up gateway tcp listener: %w", err)
	}
	defer tcpListener.Close()

	udpListener, err := listenCfg.ListenPacket(ctx.Context, "udp", listenUdp)
	if err != nil {
		return fmt.Errorf("error setting up gateway udp listener: %w", err)
	}
	defer udpListener.Close()

	var httpListener net.Listener
	if advertisePort == 443 || ctx.IsSet("listen-http") {
		httpListener, err = listenCfg.Listen(ctx.Context, "tcp", fmt.Sprintf("%s:%d", tcpHost, ctx.Int("listen-http")))
		if err != nil {
			return fmt.Errorf("error setting up http listener: %w", err)
		}
		defer httpListener.Close()
	}

	alpnMux, err := overlay.NewMux(udpListener)
	if err != nil {
		return fmt.Errorf("error setting up quic alpn muxer: %w", err)
	}
	defer alpnMux.Close()

	chordName := fmt.Sprintf("chord://%s", advertise)
	tunnelName := fmt.Sprintf("tunnel://%s", advertise)

	logger.Info("Using advertise addresses as destinations", zap.String("chord", chordName), zap.String("tunnel", tunnelName))

	chordTLS := cipher.GetPeerTLSConfig(bundle.ca, bundle.node, []string{
		tun.ALPN(protocol.Link_SPECTER_CHORD),
	})

	// handles specter-chord/1
	chordListener := alpnMux.With(chordTLS, tun.ALPN(protocol.Link_SPECTER_CHORD))
	defer chordListener.Close()

	chordRTT := rtt.NewInstrumentation(20)
	chordTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger: logger.With(zapsentry.NewScope()).With(zap.String("component", "chordTransport")),
		Endpoint: &protocol.Node{
			Address: advertise,
		},
		ClientTLS:        chordTLS,
		RTTRecorder:      chordRTT,
		VirtualTransport: true,
	})
	defer chordTransport.Stop()

	// TODO: measure rtt to client to build routing table with cost
	tunnelTransport := overlay.NewQUIC(overlay.TransportConfig{
		UseCertificateIdentity: true,
		Logger:                 logger.With(zapsentry.NewScope()).With(zap.String("component", "tunnelTransport")),
		Endpoint: &protocol.Node{
			Address: advertise,
		},
	})
	defer tunnelTransport.Stop()

	streamRouter := transport.NewStreamRouter(logger.With(zapsentry.NewScope()).With(zap.String("component", "router")), chordTransport, tunnelTransport)

	chordClient := rpc.DynamicChordClient(ctx.Context, chordTransport)
	virtualNodes := make([]*chordImpl.LocalNode, 0)

	k := ctx.Int("virtual")
	for i := 0; i < k; i++ {
		nodeIdentity := &protocol.Node{
			Id:      chord.Hash([]byte(fmt.Sprintf("%s/%d", chordName, i))),
			Address: advertise,
		}
		kvProvider, err := aof.New(aof.Config{
			Logger:        logger.With(zapsentry.NewScope()).With(zap.String("component", "kv"), zap.Object("node", nodeIdentity)),
			HasnFn:        chord.Hash,
			DataDir:       filepath.Join(ctx.String("data-dir"), fmt.Sprintf("%d", i)),
			FlushInterval: time.Second * 3,
		})
		if err != nil {
			return fmt.Errorf("initializing kv storage: %w", err)
		}
		defer kvProvider.Stop()
		go kvProvider.Start()

		virtualNode := chordImpl.NewLocalNode(chordImpl.NodeConfig{
			Identity:                 nodeIdentity,
			BaseLogger:               logger,
			ChordClient:              chordClient,
			KVProvider:               kvProvider,
			StabilizeInterval:        time.Second * 3,
			FixFingerInterval:        time.Second * 5,
			PredecessorCheckInterval: time.Second * 7,
			NodesRTT:                 chordRTT,
		})

		virtualNode.AttachRouter(ctx.Context, streamRouter)
		virtualNodes = append(virtualNodes, virtualNode)
	}

	rootNode := virtualNodes[0]
	rootNode.AttachRoot(ctx.Context, streamRouter)
	if rpcListener != nil {
		logger.Info("Exposing RPC externally", zap.String("listen", ctx.String("listen-rpc")))
		rootNode.AttachExternal(ctx.Context, rpcListener)
	}

	go chordTransport.AcceptWithListener(ctx.Context, chordListener)
	go streamRouter.Accept(ctx.Context)
	go alpnMux.Accept(ctx.Context)

	if !ctx.IsSet("join") {
		if err := rootNode.Create(); err != nil {
			return fmt.Errorf("error bootstrapping chord ring: %w", err)
		}
	} else {
		p, err := chordImpl.NewRemoteNode(ctx.Context, logger, chordClient, &protocol.Node{
			Unknown: true,
			Address: ctx.String("join"),
		})
		if err != nil {
			return fmt.Errorf("error connecting existing chord node: %w", err)
		}
		if err := rootNode.Join(p); err != nil {
			return fmt.Errorf("error joining root node to existing chord ring: %w", err)
		}
	}
	defer rootNode.Leave()

	for i := 1; i < k; i++ {
		p, err := chordImpl.NewRemoteNode(ctx.Context, logger, chordClient, rootNode.Identity())
		if err != nil {
			return fmt.Errorf("error connecting to root node: %w", err)
		}
		if err := virtualNodes[i].Join(p); err != nil {
			return fmt.Errorf("error joining virtual node to root node: %w", err)
		}
		defer virtualNodes[i].Leave()
	}

	certProvider, err := configCertProvider(ctx, logger.With(zapsentry.NewScope()), rootNode)
	if err != nil {
		return fmt.Errorf("failed to configure cert provider: %w", err)
	}

	if err := certProvider.Initialize(ctx.Context); err != nil {
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
	gwH3Listener := alpnMux.With(gwTLSConf, append([]string{tun.ALPN(protocol.Link_TCP)}, cipher.H3Protos...)...)
	defer gwH3Listener.Close()

	// handles specter-client/1
	clientTLSConf := cipher.GetClientTLSConfig(bundle.clientCa, certProvider.GetCertificate, []string{tun.ALPN(protocol.Link_SPECTER_CLIENT)})
	clientListener := alpnMux.With(clientTLSConf, tun.ALPN(protocol.Link_SPECTER_CLIENT))
	defer clientListener.Close()

	tunnelIdentity := &protocol.Node{
		Id:      chord.Hash([]byte(tunnelName)),
		Address: advertise,
	}
	tunServer := server.New(server.Config{
		Logger:          logger.With(zapsentry.NewScope()).With(zap.String("component", "tunnelServer"), zap.Uint64("node", tunnelIdentity.GetId())),
		Chord:           rootNode,
		TunnelTransport: tunnelTransport,
		ChordTransport:  chordTransport,
		Resolver:        net.DefaultResolver,
		Apex:            rootDomain,
		Acme:            ctx.String("acme_zone"),
	})
	defer tunServer.Stop()

	tunServer.AttachRouter(ctx.Context, streamRouter)
	tunServer.MustRegister(ctx.Context)

	go tunnelTransport.AcceptWithListener(ctx.Context, clientListener)

	gw := gateway.New(gateway.GatewayConfig{
		PKIServer: &pki.Server{
			Logger:   logger.With(zapsentry.NewScope()).With(zap.String("component", "pki")),
			ClientCA: bundle.clientCaCert,
		},
		Handlers: gateway.InternalHandlers{
			StatsHandler:     chordImpl.StatsHandler(virtualNodes),
			RingGraphHandler: chordImpl.RingGraphHandler(rootNode),
			MigrationHandler: migrator.ConfigMigrator(logger.With(zapsentry.NewScope()).With(zap.String("component", "migrator")), bundle.clientCaCert),
		},
		Logger:       logger.With(zapsentry.NewScope()).With(zap.String("component", "gateway")),
		TunnelServer: tunServer,
		HTTPListener: httpListener,
		H2Listener:   gwH2Listener,
		H3Listener:   gwH3Listener,
		RootDomains:  managedDomains,
		GatewayPort:  int(advertisePort),
		AdminUser:    ctx.String("auth_user"),
		AdminPass:    ctx.String("auth_pass"),
	})
	defer gw.Close()

	gw.AttachRouter(ctx.Context, streamRouter)
	gw.MustStart(ctx.Context)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigs:
		logger.Info("received signal to stop", zap.String("signal", sig.String()))
	case <-ctx.Context.Done():
		logger.Info("context done", zap.Error(ctx.Context.Err()))
	}

	return nil
}
