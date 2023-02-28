package client

import (
	"net"

	"kon.nect.sh/specter/tun/client/dialer"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func tlsDialer(ctx *cli.Context, logger *zap.Logger, parsed *dialer.ParsedApex, norebootstrap bool) (net.Addr, dialer.TransportDialer, error) {
	// used in integration test
	if v, ok := ctx.App.Metadata["connectOverride"]; ok {
		parsed.Host = v.(string)
	}

	return dialer.TLSDialer(ctx.Context, dialer.DialerConfig{
		Logger:             logger,
		Parsed:             parsed,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NoReconnection:     norebootstrap,
	})
}

func quicDialer(ctx *cli.Context, logger *zap.Logger, parsed *dialer.ParsedApex, norebootstrap bool) (net.Addr, dialer.TransportDialer, error) {
	// used in integration test
	if v, ok := ctx.App.Metadata["connectOverride"]; ok {
		parsed.Host = v.(string)
	}

	return dialer.QuicDialer(ctx.Context, dialer.DialerConfig{
		Logger:             logger,
		Parsed:             parsed,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NoReconnection:     norebootstrap,
	})
}
