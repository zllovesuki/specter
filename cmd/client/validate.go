package client

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/spec/acme"
	"kon.nect.sh/specter/tun/client"
	"kon.nect.sh/specter/tun/client/dialer"

	"github.com/quic-go/quic-go"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func cmdValidate(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	hostname := ctx.Args().First()
	if hostname == "" {
		return fmt.Errorf("missing hostname in argument")
	}

	cfg, err := client.NewConfig(ctx.String("config"))
	if err != nil {
		return err
	}

	parsed, err := dialer.ParseApex(cfg.Apex)
	if err != nil {
		return err
	}

	hostname, err = acme.Normalize(hostname)
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

	_, transport := createTransport(ctx, transportCfg{
		logger: logger,
		quicTp: quicTransport,
		apex:   parsed,
	})
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

	resp, err := c.RequestAcmeValidation(ctx.Context, hostname)
	if err != nil {
		return fmt.Errorf("failed to validate acme setup: %w", err)
	}

	fmt.Printf("\n")
	c.FormatValidate(hostname, resp, os.Stdout)

	return nil
}
