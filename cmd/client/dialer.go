package client

import (
	"net"

	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func tlsDialer(ctx *cli.Context, logger *zap.Logger, parsed *dialer.ParsedApex, norebootstrap bool) (net.Addr, dialer.TransportDialer, error) {
	// used in integration test
	dialerCtx := ctx.Context
	if v, ok := ctx.App.Metadata["connectOverride"]; ok {
		dialerCtx = dialer.WithServerNameOverride(ctx.Context, v.(string))
	}

	return dialer.TLSDialer(dialerCtx, dialer.DialerConfig{
		Logger:             logger,
		Parsed:             parsed,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NoReconnection:     norebootstrap,
	})
}

func quicDialer(ctx *cli.Context, logger *zap.Logger, parsed *dialer.ParsedApex, norebootstrap bool) (net.Addr, dialer.TransportDialer, error) {
	// used in integration test
	dialerCtx := ctx.Context
	if v, ok := ctx.App.Metadata["connectOverride"]; ok {
		dialerCtx = dialer.WithServerNameOverride(ctx.Context, v.(string))
	}

	return dialer.QuicDialer(dialerCtx, dialer.DialerConfig{
		Logger:             logger,
		Parsed:             parsed,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NoReconnection:     norebootstrap,
	})
}
