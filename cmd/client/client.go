package client

import (
	"fmt"
	"os"

	"kon.nect.sh/specter/util"

	"github.com/urfave/cli/v2"
)

var (
	// used in dev-client docker image
	devApexOverride = ""
)

func Generate() *cli.Command {
	return &cli.Command{
		Name:        "client",
		ArgsUsage:   " ",
		Usage:       "start an specter client on this machine",
		Description: "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Duis fringilla suscipit tincidunt. Aenean ut sem ipsum. ",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "insecure",
				Value: false,
				Usage: "disable TLS verification, useful for debugging and local development",
			},
		},
		Subcommands: []*cli.Command{
			{
				Name:      "tunnel",
				ArgsUsage: " ",
				Usage:     "create reverse highly available tunnels to this machine",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "config",
						Aliases:  []string{"c"},
						Usage:    "path to config yaml file.",
						Required: true,
					},
				},
				Action: cmdTunnel,
			},
			{
				Name:      "connect",
				ArgsUsage: "[hostname of the tunnel]",
				Usage:     "connect to target via stdin/stdout, usually for connecting to TCP endpoint",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "tcp",
						Usage: "fallback to connect to gateway via TLS/TCP instead of QUIC",
					},
				},
				Before: func(ctx *cli.Context) error {
					if _, ok := ctx.App.Metadata[PipeInKey]; !ok {
						ctx.App.Metadata[PipeInKey] = os.Stdin
					}
					if _, ok := ctx.App.Metadata[PipeOutKey]; !ok {
						ctx.App.Metadata[PipeOutKey] = os.Stdout
					}
					return nil
				},
				Action: cmdConnect,
			},
			{
				Name:      "listen",
				ArgsUsage: "[hostname of the tunnel]",
				Usage:     "listen for connections locally and forward them to target",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "tcp",
						Usage: "fallback to connect to gateway via TLS/TCP instead of QUIC.",
					},
					&cli.StringFlag{
						Name:        "listen",
						Aliases:     []string{"l"},
						DefaultText: fmt.Sprintf("%s:1337", util.GetOutboundIP().String()),
						Usage:       "address and port to listen for incoming TCP connections",
						Required:    true,
					},
				},
				Action: cmdListen,
			},
		},
		Before: func(ctx *cli.Context) error {
			if devApexOverride != "" {
				ctx.App.Metadata["apexOverride"] = devApexOverride
			}
			return nil
		},
	}
}
