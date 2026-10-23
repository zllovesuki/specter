package client

import (
	"context"
	"io"
	"os/signal"
	"syscall"

	"go.miragespace.co/specter/tun/client"
	"go.miragespace.co/specter/tun/client/dialer"
)

type LightweightOptions struct {
	Options
	Apex   string
	Target string
	Token  string
	Output io.Writer
}

func RunLightweight(ctx context.Context, opts LightweightOptions) error {
	apex, err := dialer.ParseApex(opts.Apex)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	tlsConfig, transport, closeTransport, err := openTransport(opts.Options, apex, nil)
	if err != nil {
		return err
	}
	defer closeTransport()
	l, err := client.NewLightweightClient(client.LightweightConfig{
		Logger:    opts.Logger,
		Transport: transport,
		PKIClient: dialer.GetPKIClient(tlsConfig.Clone(), apex),
		Apex:      apex,
		Target:    opts.Target,
		Token:     opts.Token,
		Output:    opts.Output,
	})
	if err != nil {
		return err
	}
	return l.Run(ctx)
}
