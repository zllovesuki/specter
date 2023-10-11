package client

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/tun/client"
	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/quic-go/quic-go"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func cmdAcme(ctx *cli.Context) error {
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

	resp, err := c.GetAcmeInstruction(ctx.Context, hostname)
	if err != nil {
		return fmt.Errorf("failed to query acme instruction: %w", err)
	}

	fmt.Printf("\n")
	c.FormatAcme(resp, os.Stdout)

	return nil
}
