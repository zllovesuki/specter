package client

import (
	"context"

	clientruntime "go.miragespace.co/specter/internal/client"

	"github.com/urfave/cli/v3"
)

func cmdTunnel(ctx context.Context, cmd *cli.Command) error {
	opts := clientruntime.TunnelOptions{
		Options:    runtimeOptions(cmd),
		ConfigPath: cmd.String("config"),
	}
	if cmd.IsSet("server") {
		address := cmd.String("server")
		opts.ServerAddress = &address
	}
	if cmd.IsSet("keyless") {
		address := cmd.String("keyless")
		opts.KeylessAddress = &address
	}
	return clientruntime.RunTunnel(ctx, opts)
}
