package client

import (
	"fmt"
	"strconv"
	"strings"

	"kon.nect.sh/specter/util"

	"github.com/urfave/cli/v2"
)

var Cmd = &cli.Command{
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
					Usage: "fallback to connect to gateway via TCP instead of QUIC",
				},
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
					Usage: "fallback to connect to gateway via TCP instead of QUIC. This is not recommended in listener mode.",
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
}

var (
	devApexOverride = ""
)

type parsedApex struct {
	host string
	port int
}

func (p *parsedApex) String() string {
	return fmt.Sprintf("%s:%d", p.host, p.port)
}

func parseApex(apex string) (*parsedApex, error) {
	port := 443
	i := strings.Index(apex, ":")
	if i != -1 {
		nP, err := strconv.ParseInt(apex[i+1:], 0, 32)
		if err != nil {
			return nil, fmt.Errorf("error parsing port number: %w", err)
		}
		apex = apex[:i]
		port = int(nP)
	}
	return &parsedApex{
		host: apex,
		port: port,
	}, nil
}
