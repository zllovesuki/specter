package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
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
	Name:        "server",
	Usage:       "start an specter server on the edge",
	Description: "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Duis fringilla suscipit tincidunt. Aenean ut sem ipsum. ",
	ArgsUsage:   " ",
	Flags: []cli.Flag{
		&cli.PathFlag{
			Name: "cert-dir",
			Usage: `location to directory containing ca.crt, node.crt, and node.key for mutual TLS between specter server nodes
			Warning: do not use certificates issued by public CA, otherwise anyone can join your specter network`,
			Required: true,
		},
		&cli.StringFlag{
			Name: "join",
			Usage: `a known specter server's listen-chord address.
			Absent of this flag will boostrap a new cluster with current node as the seed node`,
		},
		&cli.StringFlag{
			Name:     "apex",
			Usage:    "canonical domain to be used as tunnel root domain. Tunnels will be given names under *.`APEX`",
			Required: true,
		},
		&cli.StringFlag{
			Name:        "listen-chord",
			Aliases:     []string{"chord"},
			DefaultText: "192.168.2.1:18281",
			Usage:       "address and port to listen for incoming specter server connections",
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "listen-client",
			Aliases:     []string{"client"},
			DefaultText: "192.168.2.1:18282",
			Usage:       "address and port to listen for incoming specter client connections",
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "listen-gateway",
			Aliases:     []string{"gateway"},
			DefaultText: "192.168.2.1:18283",
			Usage:       "address and port to listen for incoming gateway connections",
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "challenger",
			DefaultText: "acme://{ACME_EMAIL}:{CF_API_TOKEN}@acmehostedzone.com",
			Usage: `to enable ACME, provide an email for issuer, the Cloudflare API token, and the Cloudflare zone responsible for hosting challanges
			Absent of this flag will serve self-signed certificate`,
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
			Name:   "cf_token",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "cf_zone",
			Hidden: true,
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
				return fmt.Errorf("missing email address in dsn")
			}
			cf, ok := parse.User.Password()
			if !ok {
				return fmt.Errorf("missing cloudflare api token in dsn")
			}
			hosted := parse.Hostname()
			if hosted == "" {
				return fmt.Errorf("missing hosted zone in dsn")
			}

			ctx.Set("email", email)
			ctx.Set("cf_token", cf)
			ctx.Set("cf_zone", hosted)

			if ds.IsDev(cipher.CertCA) {
				return nil
			} else {
				p := &cloudflare.Provider{
					APIToken: cf,
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
	caCert, err := ioutil.ReadFile(files[0])
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

	// TODO: verify they are not 0.0.0.0 or ::0
	chordIdentity := &protocol.Node{
		Id:      chordSpec.Random(),
		Address: ctx.String("listen-chord"),
	}
	serverIdentity := &protocol.Node{
		Id:      chordSpec.Random(),
		Address: ctx.String("listen-client"),
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

	tunTLSConf := cipher.GetGatewayTLSConfig(certProvider.GetCertificate, []string{
		tun.ALPN(protocol.Link_SPECTER_TUN),
	})

	// TODO: implement SNI proxy so specter can share port with another webserver
	gwListener, err := tls.Listen("tcp", ctx.String("listen-gateway"), gwTLSConf)
	if err != nil {
		return fmt.Errorf("setting up gateway listener: %w", err)
	}
	defer gwListener.Close()

	chordLogger := logger.With(zap.String("component", "chord"), zap.Uint64("node", chordIdentity.GetId()))
	tunLogger := logger.With(zap.String("component", "tun"), zap.Uint64("node", serverIdentity.GetId()))
	gwLogger := logger.With(zap.String("component", "gateway"))

	chordTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:    chordLogger,
		Endpoint:  chordIdentity,
		ServerTLS: chordTLS,
		ClientTLS: chordTLS,
	})
	defer chordTransport.Stop()

	clientTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:    tunLogger,
		Endpoint:  serverIdentity,
		ServerTLS: tunTLSConf,
		ClientTLS: nil,
	})
	defer clientTransport.Stop()

	chordNode := chord.NewLocalNode(chord.NodeConfig{
		Logger:                   chordLogger,
		Identity:                 chordIdentity,
		Transport:                chordTransport,
		KVProvider:               kv.WithChordHash(),
		FixFingerInterval:        time.Second * 3,
		StablizeInterval:         time.Second * 5,
		PredecessorCheckInterval: time.Second * 7,
	})
	defer chordNode.Stop()

	rootDomain := ctx.String("apex")
	tunServer := server.New(tunLogger, chordNode, clientTransport, chordTransport, rootDomain)
	defer tunServer.Stop()

	gwPort := gwListener.Addr().(*net.TCPAddr).Port
	clientIf, err := net.ResolveUDPAddr("udp", ctx.String("listen-client"))
	if err != nil {
		return fmt.Errorf("error parsing client listening address: %w", err)
	}
	clientPort := clientIf.Port

	gw, err := gateway.New(gateway.GatewayConfig{
		Logger:      gwLogger,
		Tun:         tunServer,
		Listener:    gwListener,
		RootDomain:  rootDomain,
		GatewayPort: gwPort,
		ClientPort:  clientPort,
	})
	if err != nil {
		return fmt.Errorf("error starting gateway server: %w", err)
	}

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

	go chordTransport.Accept(ctx.Context)
	go chordNode.HandleRPC(ctx.Context)
	go clientTransport.Accept(ctx.Context)
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
