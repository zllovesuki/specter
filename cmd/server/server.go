package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"kon.nect.sh/specter/chord"
	ds "kon.nect.sh/specter/dev-support/server"
	"kon.nect.sh/specter/kv"
	"kon.nect.sh/specter/overlay"
	chordSpec "kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/cipher"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/gateway"
	"kon.nect.sh/specter/tun/server"

	"github.com/caddyserver/certmagic"
	"github.com/mholt/acmez"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"kon.nect.sh/challenger/cloudflare"
)

var Cmd = &cli.Command{
	Name:  "server",
	Usage: "start an specter server on the edge",
	Description: `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Duis fringilla suscipit tincidunt. Aenean ut sem ipsum.

	Specter server provides an internal endpoint on /_internal under apex domain. To enable internal endpoint, provide username and password 
	under environment variables INTERNAL_USER and INTERNAL_PASS. Absent of them will disable the internal endpoint entirely.`,
	ArgsUsage: " ",
	Flags: []cli.Flag{
		&cli.PathFlag{
			Name: "cert-dir",
			Usage: `location to directory containing ca.crt, node.crt, and node.key for mutual TLS between specter server nodes
			Warning: do not use certificates issued by public CA, otherwise anyone can join your specter network`,
			Required: true,
		},
		&cli.StringFlag{
			Name: "join",
			Usage: `a known specter server's advertise address.
			Absent of this flag will boostrap a new cluster with current node as the seed node`,
		},
		&cli.StringFlag{
			Name:     "apex",
			Usage:    "canonical domain to be used as tunnel root domain. Tunnels will be given names under *.`APEX`",
			Required: true,
		},
		&cli.StringFlag{
			Name:        "listen-addr",
			Aliases:     []string{"listen"},
			DefaultText: fmt.Sprintf("%s:443", GetOutboundIP().String()),
			Usage:       "address and port to listen for specter server, specter client and gateway connections. This port will serve both TCP and UDP",
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "advertise-addr",
			Aliases:     []string{"advertise"},
			DefaultText: "same as listen-addr",
			Usage:       "address and port to advertise to specter servers and clients to connect to",
		},
		&cli.StringFlag{
			Name:        "challenger",
			DefaultText: "acme://{ACME_EMAIL}:{CF_API_TOKEN}@acmehostedzone.com",
			Usage: `to enable ACME, provide an email for issuer, the Cloudflare API token, and the Cloudflare zone responsible for hosting challanges
			Absent of this flag will serve self-signed certificate.
			Alternatively, you can set API token via the environment variable CF_API_TOKEN`,
		},
		&cli.StringFlag{
			Name:        "sentry",
			DefaultText: "https://public@sentry.example.com/1",
			Usage:       "sentry DSN for error monitoring",
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
	},
	Before: func(ctx *cli.Context) error {
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

type certBundle struct {
	ca   *x509.CertPool
	node tls.Certificate
}

func certLoader(dir string) (*certBundle, error) {
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

func configACME(ctx *cli.Context, logger *zap.Logger) *certmagic.Config {
	magic := certmagic.NewDefault()
	magic.DefaultServerName = ctx.String("apex")
	magic.Logger = logger.With(zap.String("component", "acme"))

	issuer := certmagic.NewACMEIssuer(magic, certmagic.ACMEIssuer{
		CA:                      cipher.CertCA,
		Email:                   ctx.String("email"),
		Agreed:                  true,
		Logger:                  logger.With(zap.String("component", "issuer")),
		DNS01Solver:             configSolver(ctx, logger),
		DisableHTTPChallenge:    true,
		DisableTLSALPNChallenge: true,
	})
	magic.Issuers = []certmagic.Issuer{issuer}
	return magic
}

func configCertProvider(ctx *cli.Context, logger *zap.Logger) cipher.CertProvider {
	rootDomain := ctx.String("apex")
	if ctx.IsSet("challenger") {
		logger.Info("Using certmagic as cert provider", zap.String("email", ctx.String("email")), zap.String("challenger", ctx.String("cf_zone")))
		magic := configACME(ctx, logger)
		return &ACMEProvider{
			Config: magic,
			InitializeFn: func() {
				magic.ManageAsync(ctx.Context, []string{rootDomain, "*." + rootDomain})
			},
		}
	} else {
		logger.Info("Using self-signed as cert provider")
		self := &ds.SelfSignedProvider{
			RootDomain: rootDomain,
		}
		return self
	}
}

func cmdServer(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)
	addr := ctx.String("listen-addr")
	advertise := addr
	if ctx.IsSet("advertise-addr") {
		advertise = ctx.String("advertise-addr")
	}

	chordIdentity := &protocol.Node{
		Id:      chordSpec.Random(),
		Address: advertise,
	}
	serverIdentity := &protocol.Node{
		Id:      chordSpec.Random(),
		Address: advertise,
	}

	bundle, err := certLoader(ctx.Path("cert-dir"))
	if err != nil {
		return fmt.Errorf("error loading certificates from directory: %w", err)
	}

	certProvider := configCertProvider(ctx, logger)

	chordTLS := cipher.GetPeerTLSConfig(bundle.ca, bundle.node, []string{
		tun.ALPN(protocol.Link_SPECTER_CHORD),
	})

	gwTLSConf := cipher.GetGatewayTLSConfig(certProvider.GetCertificate, []string{
		tun.ALPN(protocol.Link_HTTP2),
		tun.ALPN(protocol.Link_HTTP),
		tun.ALPN(protocol.Link_TCP),
		tun.ALPN(protocol.Link_UNKNOWN),
	})

	chordLogger := logger.With(zap.String("component", "chord"), zap.Uint64("node", chordIdentity.GetId()))
	tunLogger := logger.With(zap.String("component", "tun"), zap.Uint64("node", serverIdentity.GetId()))
	gwLogger := logger.With(zap.String("component", "gateway"))
	muxLogger := logger.With(zap.String("component", "alpnMux"))

	// TODO: implement SNI proxy so specter can share port with another webserver
	gwH2Listener, err := tls.Listen("tcp", addr, gwTLSConf)
	if err != nil {
		return fmt.Errorf("error setting up gateway http2 listener: %w", err)
	}
	defer gwH2Listener.Close()

	alpnMux, err := overlay.NewMux(muxLogger, addr)
	if err != nil {
		return fmt.Errorf("error setting up quic alpn muxer: %w", err)
	}
	defer alpnMux.Close()

	// handles h3, h3-29, and specter-tcp/1
	gwH3Listener := alpnMux.With(gwTLSConf, append(cipher.H3Protos, tun.ALPN(protocol.Link_TCP))...)

	// handles specter-tun/1
	clientListener := alpnMux.With(gwTLSConf, tun.ALPN(protocol.Link_SPECTER_TUN))

	// handles specter-chord/1
	chordListener := alpnMux.With(chordTLS, tun.ALPN(protocol.Link_SPECTER_CHORD))

	chordTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:    chordLogger,
		Endpoint:  chordIdentity,
		ClientTLS: chordTLS,
	})
	defer chordTransport.Stop()

	chordNode := chord.NewLocalNode(chord.NodeConfig{
		Logger:                   chordLogger,
		Identity:                 chordIdentity,
		Transport:                chordTransport,
		KVProvider:               kv.WithHashFn(chordSpec.HashString),
		FixFingerInterval:        time.Second * 3,
		StablizeInterval:         time.Second * 5,
		PredecessorCheckInterval: time.Second * 7,
	})
	defer chordNode.Stop()

	clientTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:   tunLogger,
		Endpoint: serverIdentity,
	})
	defer clientTransport.Stop()

	rootDomain := ctx.String("apex")
	tunServer := server.New(tunLogger, chordNode, clientTransport, chordTransport, rootDomain)
	defer tunServer.Stop()

	// TODO: use advertise?
	gwPort := gwH2Listener.Addr().(*net.TCPAddr).Port
	gw, err := gateway.New(gateway.GatewayConfig{
		Logger:       gwLogger,
		Tun:          tunServer,
		H2Listener:   gwH2Listener,
		H3Listener:   gwH3Listener,
		StatsHandler: chordNode.StatsHandler,
		RootDomain:   rootDomain,
		GatewayPort:  gwPort,
		AdminUser:    ctx.String("auth_user"),
		AdminPass:    ctx.String("auth_pass"),
	})
	if err != nil {
		return fmt.Errorf("error starting gateway server: %w", err)
	}
	defer gw.Close()

	if !ctx.IsSet("join") {
		if err := chordNode.Create(); err != nil {
			return fmt.Errorf("error bootstrapping chord ring: %w", err)
		}
	} else {
		p, err := chord.NewRemoteNode(ctx.Context, chordTransport, chordLogger, &protocol.Node{
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

	certProvider.Initialize(chordNode)

	go alpnMux.Accept(ctx.Context)
	go chordTransport.AcceptWithListener(ctx.Context, chordListener)
	go chordNode.HandleRPC(ctx.Context)
	go clientTransport.AcceptWithListener(ctx.Context, clientListener)
	go tunServer.HandleRPC(ctx.Context)
	go tunServer.Accept(ctx.Context)
	go gw.Start(ctx.Context)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("received signal to stop", zap.String("signal", (<-sigs).String()))

	return nil
}

type ACMEProvider struct {
	*certmagic.Config
	InitializeFn func()
}

func (a *ACMEProvider) Initialize(node chordSpec.VNode) {
	a.InitializeFn()
}

var _ cipher.CertProvider = (*ACMEProvider)(nil)

func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "1.1.1.1:53")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}
