package client

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.miragespace.co/specter/tun/client"
	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/quic-go/quic-go"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func cmdLs(ctx context.Context, cmd *cli.Command) error {
	logger := cmd.Root().Metadata["logger"].(*zap.Logger)

	cfg, err := client.NewConfig(cmd.String("config"))
	if err != nil {
		return err
	}

	parsed, err := dialer.ParseApex(cfg.Apex)
	if err != nil {
		return err
	}

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)

	listener, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return err
	}
	defer listener.Close()

	quicTransport := &quic.Transport{Conn: listener}
	defer quicTransport.Close()

	_, transport := createTransport(cmd, transportCfg{
		logger: logger,
		quicTp: quicTransport,
		apex:   parsed,
	})
	defer transport.Stop()

	c, err := client.NewClient(ctx, client.ClientConfig{
		Logger:          logger,
		Configuration:   cfg,
		ServerTransport: transport,
		ReloadSignal:    s,
	})
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Register(ctx); err != nil {
		return err
	}

	hostnames, err := c.GetRegisteredHostnames(ctx)
	if err != nil {
		return err
	}

	c.FormatList(hostnames, os.Stdout)

	return nil
}
