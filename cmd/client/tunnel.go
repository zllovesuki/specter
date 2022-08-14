package client

import (
	"crypto/tls"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/overlay"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/client"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func createTransport(ctx *cli.Context, logger *zap.Logger, cfg *client.Config, apex *parsedApex) *overlay.QUIC {
	clientTLSConf := &tls.Config{
		ServerName:         apex.host,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NextProtos: []string{
			tun.ALPN(protocol.Link_SPECTER_TUN),
		},
	}
	if v, ok := ctx.App.Metadata["apexOverride"]; ok {
		clientTLSConf.ServerName = v.(string)
	}
	return overlay.NewQUIC(overlay.TransportConfig{
		Logger: logger,
		Endpoint: &protocol.Node{
			Id: cfg.ClientID,
		},
		ClientTLS: clientTLSConf,
	})
}

func cmdTunnel(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	cfg, err := client.NewConfig(ctx.String("config"))
	if err != nil {
		return err
	}

	parsed, err := parseApex(cfg.Apex)
	if err != nil {
		return err
	}

	transport := createTransport(ctx, logger, cfg, parsed)
	defer transport.Stop()

	c, err := client.NewClient(ctx.Context, logger, transport, cfg)
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Register(ctx.Context); err != nil {
		return err
	}

	if err := c.Initialize(ctx.Context); err != nil {
		return err
	}

	go c.Accept(ctx.Context)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("received signal to stop", zap.String("signal", (<-sigs).String()))

	return nil
}
