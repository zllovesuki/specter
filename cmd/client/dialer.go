package client

import (
	"context"
	"net"

	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func tlsDialer(ctx context.Context, cmd *cli.Command, logger *zap.Logger, parsed *dialer.ParsedApex, norebootstrap bool) (net.Addr, dialer.TransportDialer, error) {
	// used in integration test
	dialerCtx := ctx
	if v, ok := cmd.Root().Metadata["connectOverride"]; ok {
		dialerCtx = dialer.WithServerNameOverride(ctx, v.(string))
	}

	return dialer.TLSDialer(dialerCtx, dialer.DialerConfig{
		Logger:             logger,
		Parsed:             parsed,
		InsecureSkipVerify: cmd.Bool("insecure"),
		NoReconnection:     norebootstrap,
	})
}

func quicDialer(ctx context.Context, cmd *cli.Command, logger *zap.Logger, parsed *dialer.ParsedApex, norebootstrap bool) (net.Addr, dialer.TransportDialer, error) {
	// used in integration test
	dialerCtx := ctx
	if v, ok := cmd.Root().Metadata["connectOverride"]; ok {
		dialerCtx = dialer.WithServerNameOverride(ctx, v.(string))
	}

	return dialer.QuicDialer(dialerCtx, dialer.DialerConfig{
		Logger:             logger,
		Parsed:             parsed,
		InsecureSkipVerify: cmd.Bool("insecure"),
		NoReconnection:     norebootstrap,
	})
}
