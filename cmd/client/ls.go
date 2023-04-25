package client

import (
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/tun/client"
	"kon.nect.sh/specter/tun/client/dialer"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func cmdLs(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	cfg, err := client.NewConfig(ctx.String("config"))
	if err != nil {
		return err
	}

	parsed, err := dialer.ParseApex(cfg.Apex)
	if err != nil {
		return err
	}

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)

	_, transport := createTransport(ctx, logger, cfg, parsed, nil)
	defer transport.Stop()

	c, err := client.NewClient(ctx.Context, client.ClientConfig{
		Logger:          logger,
		Configuration:   cfg,
		ServerTransport: transport,
		ReloadSignal:    s,
	})
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Register(ctx.Context); err != nil {
		return err
	}

	hostnames, err := c.GetRegisteredHostnames(ctx.Context)
	if err != nil {
		return err
	}

	c.FormatList(hostnames, os.Stdout)

	return nil
}
