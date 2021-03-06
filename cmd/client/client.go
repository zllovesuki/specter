package client

import (
	"crypto/tls"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/overlay"
	chordSpec "kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/client"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

var Cmd = &cli.Command{
	Name:        "client",
	Usage:       "start an specter client on this machine",
	Description: "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Duis fringilla suscipit tincidunt. Aenean ut sem ipsum. ",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "insecure",
			Value: false,
			Usage: "disable TLS verification, useful for debugging and local development",
		},
	},
	Action: cmdClient,
}

func cmdClient(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	self := &protocol.Node{
		Id: chordSpec.Random(),
	}

	// TODO: endpoint discovery via http
	seed := &protocol.Node{
		Address: "dev.con.nect.sh:1112",
	}

	clientTLSConf := &tls.Config{
		InsecureSkipVerify: ctx.Bool("insecure"),
		NextProtos: []string{
			tun.ALPN(protocol.Link_SPECTER_TUN),
		},
	}
	transport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:    logger,
		Endpoint:  self,
		ServerTLS: nil,
		ClientTLS: clientTLSConf,
	})
	defer transport.Stop()

	c, err := client.NewClient(ctx.Context, logger, transport, seed)
	if err != nil {
		logger.Fatal("starting new tun client", zap.Error(err))
	}

	nodes, err := c.GetCandidates(ctx.Context)
	if err != nil {
		logger.Fatal("starting new tun client", zap.Error(err))
	}

	for _, node := range nodes {
		fmt.Printf("%+v\n", node)
	}

	for _, node := range nodes[1:] {
		_, err := transport.DialDirect(ctx.Context, node)
		if err != nil {
			logger.Error("connecting to specter node", zap.Error(err))
		}
	}

	hostname, err := c.PublishTunnel(ctx.Context, nodes)
	if err != nil {
		logger.Fatal("publishing tunnel", zap.Error(err))
	}

	logger.Info("tunnel published", zap.String("hostname", hostname))

	go c.Tunnel(ctx.Context, hostname)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("received signal to stop", zap.String("signal", (<-sigs).String()))

	return nil
}
