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

	"kon.nect.sh/specter/acme/storage"
	chordImpl "kon.nect.sh/specter/chord"
	ds "kon.nect.sh/specter/dev/server"
	"kon.nect.sh/specter/kv/aof"
	"kon.nect.sh/specter/overlay"
	"kon.nect.sh/specter/rtt"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/cipher"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/gateway"
	"kon.nect.sh/specter/tun/server"
	"kon.nect.sh/specter/util"
	"kon.nect.sh/specter/util/router"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/caddyserver/certmagic"
	"github.com/getsentry/sentry-go"
	"github.com/mholt/acmez"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"kon.nect.sh/challenger/cloudflare"
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
				Name:     "apex",
				Usage:    "canonical domain to be used as tunnel root domain. Tunnels will be given names under *.`APEX`",
				Required: true,
			},
			&cli.PathFlag{
				Name:     "data-dir",
				Aliases:  []string{"data"},
				Usage:    "path to directory that will be used for persisting non-volatile KV data",
				Required: true,
			},
			&cli.PathFlag{
				Name:    "cert-dir",
				Aliases: []string{"cert"},
				Usage:   `path to directory containing ca.crt, node.crt, and node.key for mutual TLS between specter server nodes`,
			},
			&cli.BoolFlag{
				Name: "cert-env",
				Usage: `load ca.crt (CERT_CA), node.crt (CERT_NODE), and node.key (CERT_NODE_KEY) from environment variables encoded as base64.
			This can be set instead of loading from cert-dir. Required if environment prefers loading secrets from ENV, such as on fly.io`,
			},
			&cli.StringFlag{
				Name:    "listen-addr",
				Aliases: []string{"listen"},
				Value:   fmt.Sprintf("%s:443", ip.String()),
				Usage: `address and port to listen for specter server, specter client and gateway connections. This port will serve both TCP and UDP (unless overriden).
			Note that if specter is listening on port 443, it will also listen on port 80 to redirect http to https`,
			},
			&cli.StringFlag{
				Name:        "listen-tcp",
				DefaultText: "same as listen-addr",
				Usage:       "override the listen address and port for TCP",
			},
			&cli.StringFlag{
				Name:        "listen-udp",
				DefaultText: "same as listen-addr",
				Usage:       "override the listen address and port for UDP. Required if environment needs a specific address, such as on fly.io",
			},
			&cli.StringFlag{
				Name:        "advertise-addr",
				Aliases:     []string{"advertise"},
				DefaultText: "same as listen-addr",
				Value:       fmt.Sprintf("%s:443", ip.String()),
				Usage: `address and port to advertise to specter servers and clients to connect to.
			Note that specter will use advertised address to derive its Identity hash.`,
			},
			&cli.StringFlag{
				Name: "join",
				Usage: `a known specter server's advertise address.
			Absent of this flag will boostrap a new cluster with current node as the seed node`,
			},
			&cli.StringFlag{
				Name:        "challenger",
				DefaultText: "acme://{ACME_EMAIL}:{CF_API_TOKEN}@acmehostedzone.com",
				EnvVars:     []string{"ACME_URI"},
				Usage: `to enable ACME, provide an email for issuer, the Cloudflare API token, and the Cloudflare zone responsible for hosting challenges
			Absent of this flag will serve self-signed certificate.
			Alternatively, you can set the URI and API token via the environment variable ACME_URI and CF_API_TOKEN, respectively`,
			},
			&cli.StringFlag{
				Name:        "sentry",
				DefaultText: "https://public@sentry.example.com/1",
				Usage:       "sentry DSN for error monitoring. Alternatively, you can set the DSN via the environment variable SENTRY_DSN",
				EnvVars:     []string{"SENTRY_DSN"},
			},

			// used for acme setup internally
			&cli.StringFlag{
				Name:   "email",
				Hidden: true,
			},
			&cli.StringFlag{
				Name:    "cf_token",
				Hidden:  true,
				EnvVars: []string{"CF_API_TOKEN"},
			},
			&cli.StringFlag{
				Name:   "cf_zone",
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
		},
		Before: func(ctx *cli.Context) error {
			if !ctx.IsSet("cert-dir") && !ctx.IsSet("cert-env") {
				return fmt.Errorf("no certificate loader is specified")
			}
			if ctx.IsSet("challenger") {
				parse, err := url.Parse(ctx.String("challenger"))
				if err != nil {
					return fmt.Errorf("error parsing challenger uri: %w", err)
				}
				email := parse.User.Username()
				if email == "" {
					return fmt.Errorf("missing email address")
				}
				if !ctx.IsSet("cf_token") {
					cf, ok := parse.User.Password()
					if !ok {
						return fmt.Errorf("missing cloudflare api token")
					}
					ctx.Set("cf_token", cf)
				}
				hosted := parse.Hostname()
				if hosted == "" {
					return fmt.Errorf("missing hosted zone")
				}

				ctx.Set("email", email)
				ctx.Set("cf_zone", hosted)

				if ds.IsDev(cipher.CertCA) {
					return nil
				} else {
					p := &cloudflare.Provider{
						APIToken: ctx.String("cf_token"),
						RootZone: hosted,
					}
					if err := p.Validate(); err != nil {
						return fmt.Errorf("error validating zone on cloudflare: %w", err)
					}
					return nil
				}
			}
			return nil
		},
		Action: cmdServer,
	}
}

type certBundle struct {
	ca   *x509.CertPool
	node tls.Certificate
}

func certLoaderFilesystem(dir string) (*certBundle, error) {
	files := []string{"ca.crt", "node.crt", "node.key"}
	for i, name := range files {
		files[i] = filepath.Join(dir, name)
	}
	caCert, err := os.ReadFile(files[0])
	if err != nil {
		return nil, fmt.Errorf("reading ca bundle from file: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("unable to use provided ca bundle")
	}
	nodeCert, err := tls.LoadX509KeyPair(files[1], files[2])
	if err != nil {
		return nil, fmt.Errorf("reading cert/key from files: %w", err)
	}
	return &certBundle{
		ca:   caCertPool,
		node: nodeCert,
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
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("unable to use provided ca bundle")
	}
	pair, err := tls.X509KeyPair(nodeCert, nodeKey)
	if err != nil {
		return nil, fmt.Errorf("creating cert/key from env: %w", err)
	}
	return &certBundle{
		ca:   caCertPool,
		node: pair,
	}, nil
}

func configSolver(ctx *cli.Context, logger *zap.Logger) acmez.Solver {
	if ds.IsDev(cipher.CertCA) {
		return &ds.NoopSolver{Logger: logger}
	} else {
		return &certmagic.DNS01Solver{
			DNSProvider: &cloudflare.Provider{
				APIToken: ctx.String("cf_token"),
				RootZone: ctx.String("cf_zone"),
			},
		}
	}
}

func configCertProvider(ctx *cli.Context, logger *zap.Logger, kv chord.KV) (cipher.CertProvider, error) {
	rootDomain := ctx.String("apex")
	if ctx.IsSet("challenger") {
		kvStore, err := storage.New(logger.With(zap.String("component", "storage")), kv, storage.Config{
			RetryInterval: time.Second * 3,
			LeaseTTL:      time.Minute,
		})
		if err != nil {
			return nil, err
		}

		cfg := certmagic.Config{
			Storage:           kvStore,
			DefaultServerName: ctx.String("apex"),
			Logger:            logger.With(zap.String("component", "acme")),
		}
		if ds.IsDev(cipher.CertCA) {
			cfg.OCSP = certmagic.OCSPConfig{
				DisableStapling: true,
			}
		}
		issuer := certmagic.NewACMEIssuer(&cfg, certmagic.ACMEIssuer{
			CA:                      cipher.CertCA,
			Email:                   ctx.String("email"),
			Agreed:                  true,
			Logger:                  logger.With(zap.String("component", "acme_issuer")),
			DNS01Solver:             configSolver(ctx, logger),
			DisableHTTPChallenge:    true,
			DisableTLSALPNChallenge: true,
		})
		cfg.Issuers = []certmagic.Issuer{issuer}

		logger.Info("Using certmagic as cert provider", zap.String("email", ctx.String("email")), zap.String("challenger", ctx.String("cf_zone")))

		cache := certmagic.NewCache(certmagic.CacheOptions{
			GetConfigForCert: func(c certmagic.Certificate) (*certmagic.Config, error) {
				return &cfg, nil
			},
			Logger: logger.With(zap.String("component", "acme_cache")),
		})

		magic := certmagic.New(cache, cfg)
		return &ACMEProvider{
			Config: magic,
			InitializeFn: func(kv chord.KV) error {
				if err := magic.ManageAsync(ctx.Context, []string{rootDomain, "*." + rootDomain}); err != nil {
					logger.Error("error initializing certmagic", zap.Error(err))
					return err
				}
				return nil
			},
		}, nil
	} else {
		logger.Info("Using self-signed as cert provider")
		self := &ds.SelfSignedProvider{
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

	return logger.With(zapsentry.NewScope())
}

func cmdServer(ctx *cli.Context) error {
	logger, ok := ctx.App.Metadata["logger"].(*zap.Logger)
	if !ok || logger == nil {
		return fmt.Errorf("unable to obtain logger from app context")
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
	tcpHost, tcpPort, err := net.SplitHostPort(listenTcp)
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

	listenCfg := &net.ListenConfig{}

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
	if tcpPort == "443" {
		httpListener, err = listenCfg.Listen(ctx.Context, "tcp", fmt.Sprintf("%s:80", tcpHost))
		if err != nil {
			return fmt.Errorf("error setting up http (80) listener: %w", err)
		}
		defer httpListener.Close()
	}

	// TODO: make these less dependent on changeable parameters
	chordName := fmt.Sprintf("chord://%s", advertise)
	tunnelName := fmt.Sprintf("tunnel://%s", advertise)

	logger.Info("Using advertise addresses as identities", zap.String("chord", chordName), zap.String("tunnel", tunnelName))

	chordIdentity := &protocol.Node{
		Id:      chord.Hash([]byte(chordName)),
		Address: advertise,
	}
	tunnelIdentity := &protocol.Node{
		Id:      chord.Hash([]byte(tunnelName)),
		Address: advertise,
	}

	chordTLS := cipher.GetPeerTLSConfig(bundle.ca, bundle.node, []string{
		tun.ALPN(protocol.Link_SPECTER_CHORD),
	})

	chordLogger := logger.With(zap.String("component", "chord"), zap.Uint64("node", chordIdentity.GetId()))
	tunnelLogger := logger.With(zap.String("component", "tunnel"), zap.Uint64("node", tunnelIdentity.GetId()))

	alpnMux, err := overlay.NewMux(udpListener)
	if err != nil {
		return fmt.Errorf("error setting up quic alpn muxer: %w", err)
	}
	defer alpnMux.Close()

	go alpnMux.Accept(ctx.Context)

	// handles specter-chord/1
	chordListener := alpnMux.With(chordTLS, tun.ALPN(protocol.Link_SPECTER_CHORD))
	defer chordListener.Close()

	chordRTT := rtt.NewInstrumentation(20)
	chordTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:      chordLogger,
		Endpoint:    chordIdentity,
		ClientTLS:   chordTLS,
		RTTRecorder: chordRTT,
	})
	defer chordTransport.Stop()

	tunnelTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:   tunnelLogger,
		Endpoint: tunnelIdentity,
	})
	defer tunnelTransport.Stop()

	kvProvider, err := aof.New(aof.Config{
		Logger:        logger.With(zap.String("component", "kv")),
		HasnFn:        chord.Hash,
		DataDir:       ctx.String("data-dir"),
		FlushInterval: time.Second,
	})
	if err != nil {
		return fmt.Errorf("initializing kv storage: %w", err)
	}
	defer kvProvider.Stop()

	go kvProvider.Start()

	streamRouter := router.NewStreamRouter(logger.With(zap.String("component", "router")), chordTransport, tunnelTransport)
	go streamRouter.Accept(ctx.Context)

	chordClient := rpc.DynamicChordClient(ctx.Context, chordTransport)

	chordNode := chordImpl.NewLocalNode(chordImpl.NodeConfig{
		Logger:                   chordLogger,
		ChordClient:              chordClient,
		Identity:                 chordIdentity,
		KVProvider:               kvProvider,
		FixFingerInterval:        time.Second * 3,
		StablizeInterval:         time.Second * 5,
		PredecessorCheckInterval: time.Second * 7,
		NodesRTT:                 chordRTT,
	})
	defer chordNode.Leave()

	chordNode.AttachRouter(ctx.Context, streamRouter)
	go chordTransport.AcceptWithListener(ctx.Context, chordListener)

	if !ctx.IsSet("join") {
		if err := chordNode.Create(); err != nil {
			return fmt.Errorf("error bootstrapping chord ring: %w", err)
		}
	} else {
		p, err := chordImpl.NewRemoteNode(ctx.Context, chordLogger, chordClient, &protocol.Node{
			Unknown: true,
			Address: ctx.String("join"),
		})
		if err != nil {
			return fmt.Errorf("error connecting existing chord node: %w", err)
		}
		if err := chordNode.Join(p); err != nil {
			return fmt.Errorf("error joining to existing chord ring: %w", err)
		}
	}

	certProvider, err := configCertProvider(ctx, logger, chordNode)
	if err != nil {
		return fmt.Errorf("failed to configure cert provider: %w", err)
	}

	if err := certProvider.Initialize(chordNode); err != nil {
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
	gwH3Listener := alpnMux.With(gwTLSConf, append(cipher.H3Protos, tun.ALPN(protocol.Link_TCP))...)
	defer gwH3Listener.Close()

	// handles specter-tun/1
	clientListener := alpnMux.With(gwTLSConf, tun.ALPN(protocol.Link_SPECTER_TUN))
	defer clientListener.Close()

	rootDomain := ctx.String("apex")
	tunServer := server.New(tunnelLogger, chordNode, tunnelTransport, chordTransport, rootDomain)
	defer tunServer.Stop()

	tunServer.AttachRouter(ctx.Context, streamRouter)
	tunServer.MustRegister(ctx.Context)

	go tunnelTransport.AcceptWithListener(ctx.Context, clientListener)

	gw := gateway.New(gateway.GatewayConfig{
		Logger:         logger.With(zap.String("component", "gateway")),
		Tun:            tunServer,
		HTTPListener:   httpListener,
		H2Listener:     gwH2Listener,
		H3Listener:     gwH3Listener,
		StatsHandler:   chordNode.StatsHandler,
		RootDomain:     rootDomain,
		GatewayPort:    int(advertisePort),
		AdminUser:      ctx.String("auth_user"),
		AdminPass:      ctx.String("auth_pass"),
		IdentityGetter: tunnelTransport.Identity,
	})
	defer gw.Close()

	go gw.Start(ctx.Context)

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

type ACMEProvider struct {
	*certmagic.Config
	InitializeFn func(chord.KV) error
}

func (a *ACMEProvider) Initialize(node chord.KV) error {
	return a.InitializeFn(node)
}

var _ cipher.CertProvider = (*ACMEProvider)(nil)
