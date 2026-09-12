package client

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func serveCommand() *cli.Command {
	return &cli.Command{
		Name:      "serve",
		Usage:     "Serve a registered hostname using a domain token",
		ArgsUsage: "<target>",
		Flags: append(lightweightFlags(), &cli.StringFlag{
			Name:  "token-file",
			Usage: "read the bearer token from this file (or use SPECTER_TUNNEL_TOKEN)",
		}),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			env, set := os.LookupEnv("SPECTER_TUNNEL_TOKEN")
			if cmd.IsSet("token-file") && set {
				return fmt.Errorf("specify exactly one of --token-file or SPECTER_TUNNEL_TOKEN")
			}
			token, err := loadTunnelToken(cmd.String("token-file"), env, set)
			if err != nil {
				return err
			}
			return runLightweight(ctx, cmd, token)
		},
	}
}
