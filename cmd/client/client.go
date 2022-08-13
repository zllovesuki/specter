package client

import (
	"github.com/urfave/cli/v2"
)

var Cmd = &cli.Command{
	Name:        "client",
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
	},
}
