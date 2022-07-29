package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"kon.nect.sh/specter/chord"
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
)

var (
	CertCA = certmagic.LetsEncryptProductionCA
)

var Server = &cli.Command{
	Name:        "server",
	Usage:       "start an specter server on the edge",
	Description: "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Duis fringilla suscipit tincidunt. Aenean ut sem ipsum. ",
	Flags: []cli.Flag{
		&cli.PathFlag{
			Name:     "cert-dir",
			Aliases:  []string{"cert"},
			Usage:    "location to directory containing ca.crt, node.crt, and node.key for mutual TLS between Chord nodes",
			Required: true,
		},
		&cli.StringFlag{
			Name:    "join",
			Aliases: []string{"j"},
			Usage:   "a known specter server's listen-chord address. Absent of this flag will boostrap a new cluster with current node as the seed node",
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
			Name:        "zone",
			Aliases:     []string{"z"},
			DefaultText: "example.com",
			Usage:       "canonical domain to be used as tunnel root zone. Tunnels will be given names under *.`ZONE`",
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "email",
			DefaultText: "acme@example.com",
			Usage:       "email address for ACME issurance",
			Required:    true,
		},
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
	switch CertCA {
	case certmagic.LetsEncryptProductionCA:
		return nil
	default:
		return &NoopSolver{logger: logger}
	}
}

func configACME(ctx *cli.Context, logger *zap.Logger) *certmagic.Config {
	gg := certmagic.NewDefault()
	gg.DefaultServerName = ctx.String("zone")
	gg.Logger = logger.With(zap.String("component", "acme"))

	issuer := certmagic.NewACMEIssuer(gg, certmagic.ACMEIssuer{
		CA:          CertCA,
		Email:       ctx.String("email"),
		Agreed:      true,
		DNS01Solver: configSolver(ctx, logger),
		Logger:      logger.With(zap.String("component", "issuer")),
	})
	gg.Issuers = []certmagic.Issuer{issuer}
	return gg
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
		return fmt.Errorf("loading certificates from directory: %w", err)
	}

	rootDomain := ctx.String("zone")
	magic := configACME(ctx, logger)
	magic.ManageAsync(ctx.Context, []string{rootDomain, "*." + rootDomain})

	chordTLS := cipher.GetPeerTLSConfig(bundle.ca, bundle.node, []string{
		tun.ALPN(protocol.Link_SPECTER_CHORD),
	})

	gwTLSConf := cipher.GetGatewayTLSConfig(magic.GetCertificate, []string{
		tun.ALPN(protocol.Link_HTTP2),
		tun.ALPN(protocol.Link_HTTP),
		tun.ALPN(protocol.Link_TCP),
		tun.ALPN(protocol.Link_UNKNOWN),
	})

	tunTLSConf := cipher.GetGatewayTLSConfig(magic.GetCertificate, []string{
		tun.ALPN(protocol.Link_SPECTER_TUN),
	})

	// TODO: implement SNI proxy so specter can share port with another webserver
	gwListener, err := tls.Listen("tcp", ctx.String("listen-gateway"), gwTLSConf)
	if err != nil {
		return fmt.Errorf("setting up gateway listener: %w", err)
	}
	defer gwListener.Close()

	gwPort := gwListener.Addr().(*net.TCPAddr).Port

	chordLogger := logger.With(zap.String("component", "chord"))
	tunLogger := logger.With(zap.String("component", "tun"))
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

	tunServer := server.New(tunLogger, chordNode, clientTransport, chordTransport, rootDomain)
	defer tunServer.Stop()

	gw, err := gateway.New(gateway.GatewayConfig{
		Logger:      gwLogger,
		Tun:         tunServer,
		Listener:    gwListener,
		RootDomain:  rootDomain,
		GatewayPort: gwPort,
	})
	if err != nil {
		return fmt.Errorf("starting gateway server: %w", err)
	}

	if !ctx.IsSet("join") {
		if err := chordNode.Create(); err != nil {
			return fmt.Errorf("bootstrapping chord ring: %w", err)
		}
	} else {
		p, err := chord.NewRemoteNode(ctx.Context, chordTransport, chordLogger, &protocol.Node{
			Unknown: true,
			Address: ctx.String("join"),
		})
		if err != nil {
			return fmt.Errorf("connecting existing chord node: %w", err)
		}
		if err := chordNode.Join(p); err != nil {
			return fmt.Errorf("joining to existing chord ring: %w", err)
		}
	}

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
