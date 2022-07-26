package cmd

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kon.nect.sh/specter/chord"
	"kon.nect.sh/specter/kv"
	"kon.nect.sh/specter/overlay"
	chordSpec "kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/gateway"
	"kon.nect.sh/specter/tun/server"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

var Server = &cli.Command{
	Name:        "server",
	Usage:       "start an specter server on the edge",
	Description: "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Duis fringilla suscipit tincidunt. Aenean ut sem ipsum. ",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "bootstrap",
			Aliases: []string{"b"},
			Value:   false,
			Usage:   "bootstrap a new specter cluster with current node as the seed node. Mutually exclusive with join",
		},
		&cli.StringFlag{
			Name:    "join",
			Aliases: []string{"j"},
			Value:   "192.168.1.1:18281",
			Usage:   "a known specter server's listen-chord address",
		},
		&cli.StringFlag{
			Name:    "listen-chord",
			Aliases: []string{"chord"},
			Value:   "192.168.2.1:18281",
			Usage:   "address and port to listen for incoming specter server connections",
		},
		&cli.StringFlag{
			Name:     "listen-client",
			Aliases:  []string{"client"},
			Value:    "192.168.2.1:18282",
			Usage:    "address and port to listen for incoming specter client connections",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "listen-gateway",
			Aliases:  []string{"gateway"},
			Value:    "192.168.2.1:18283",
			Usage:    "address and port to listen for incoming gateway connections",
			Required: true,
		},
	},
	Before: func(ctx *cli.Context) error {
		if ctx.Bool("bootstrap") && ctx.IsSet("join") {
			return fmt.Errorf("cannot set bootstrap and join at the same time")
		}
		return nil
	},
	Action: cmdServer,
}

func cmdServer(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	chordIdentity := &protocol.Node{
		Id:      chordSpec.Random(),
		Address: ctx.String("listen-chord"),
	}
	serverIdentity := &protocol.Node{
		Id:      chordSpec.Random(),
		Address: ctx.String("listen-client"),
	}

	// ========== TODO: THESE TLS CONFIGS ARE FOR DEVELOPMENT ONLY ==========

	chordTLS := generateTLSConfig()
	chordTLS.NextProtos = []string{
		tun.ALPN(protocol.Link_SPECTER_CHORD),
	}
	chordClientTLS := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos: []string{
			tun.ALPN(protocol.Link_SPECTER_CHORD),
		},
	}

	gwTLSConf := generateTLSConfig()
	gwTLSConf.NextProtos = []string{
		tun.ALPN(protocol.Link_HTTP),
		tun.ALPN(protocol.Link_TCP),
		tun.ALPN(protocol.Link_UNKNOWN),
	}
	tunTLSConf := generateTLSConfig()
	tunTLSConf.NextProtos = []string{
		tun.ALPN(protocol.Link_SPECTER_TUN),
	}

	gwListener, err := tls.Listen("tcp", ctx.String("listen-gateway"), gwTLSConf)
	if err != nil {
		return fmt.Errorf("setting up gateway listener: %w", err)
	}
	defer gwListener.Close()

	gwPort := gwListener.Addr().(*net.TCPAddr).Port
	rootDomain := "example.com"

	chordLogger := logger.With(zap.String("component", "chord"))
	tunLogger := logger.With(zap.String("component", "tun"))
	gwLogger := logger.With(zap.String("component", "gateway"))

	chordTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:    chordLogger,
		Endpoint:  chordIdentity,
		ServerTLS: chordTLS,
		ClientTLS: chordClientTLS,
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

	go chordTransport.Accept(ctx.Context)
	go chordNode.HandleRPC(ctx.Context)

	if ctx.Bool("bootstrap") {
		if err := chordNode.Create(); err != nil {
			logger.Fatal("Start LocalNode with new Chord Ring", zap.Error(err))
		}
	} else {
		p, err := chord.NewRemoteNode(ctx.Context, chordTransport, chordLogger, &protocol.Node{
			Unknown: true,
			Address: ctx.String("join"),
		})
		if err != nil {
			logger.Fatal("Creating RemoteNode", zap.Error(err))
		}
		if err := chordNode.Join(p); err != nil {
			logger.Fatal("Start LocalNode with existing Chord Ring", zap.Error(err))
		}
	}

	go clientTransport.Accept(ctx.Context)
	go tunServer.HandleRPC(ctx.Context)
	go tunServer.Accept(ctx.Context)
	go gw.Start(ctx.Context)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("received signal to stop", zap.String("signal", (<-sigs).String()))

	return nil
}
