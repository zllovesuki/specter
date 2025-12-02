package client

import (
	"fmt"

	"go.miragespace.co/specter/util"

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
		Description: "Run a Specter client to publish and manage reverse tunnels over QUIC, fetch hostnames and TLS certs via ACME, and connect or listen to your endpoints",
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
				Usage:     "Create highly available reverse tunnels to this machine",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "config",
						Aliases:  []string{"c"},
						Usage:    "path to config yaml file.",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "server",
						Usage: "start local server to handle client commands while the client is running",
					},
					&cli.StringFlag{
						Name: "keyless",
						Usage: `start a keyless TLS proxy while the client is running. This is useful for local access with split DNS, without going to the edge specter node for proxying.
			Note that if keyless proxy is listening on port 443, it will also listen on port 80 to handle http connect proxy, and redirect other http requests to https`,
					},
				},
				Action: cmdTunnel,
			},
			{
				Name:      "config-example",
				ArgsUsage: " ",
				Usage:     "Print an example client configuration in YAML format",
				Action:    cmdConfigExample,
			},
			{
				Name:      "connect",
				ArgsUsage: "[hostname of the tunnel]",
				Usage:     "Connect to target via stdin/stdout, usually for connecting to TCP endpoint",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "tcp",
						Usage: "fallback to connect to gateway via TLS/TCP instead of QUIC",
					},
				},
				Action: cmdConnect,
			},
			{
				Name:      "listen",
				ArgsUsage: "[hostname of the tunnel]",
				Usage:     "Listen for connections locally and forward them to target",
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
			{
				Name:      "ls",
				ArgsUsage: " ",
				Usage:     "Query registered hostnames associated with this client",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "config",
						Aliases:  []string{"c"},
						Usage:    "path to config yaml file.",
						Required: true,
					},
				},
				Action: cmdLs,
			},
			{
				Name:      "acme",
				ArgsUsage: "[custom hostname to be used]",
				Usage:     "Obtain instructions to setup custom hostname to be used with specter",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "config",
						Aliases:  []string{"c"},
						Usage:    "path to config yaml file.",
						Required: true,
					},
				},
				Action: cmdAcme,
			},
			{
				Name:      "validate",
				ArgsUsage: "[custom hostname to be used]",
				Usage:     "Validate hostname DNS setup to be used as custom hostname",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "config",
						Aliases:  []string{"c"},
						Usage:    "path to config yaml file.",
						Required: true,
					},
				},
				Action: cmdValidate,
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
