package dns

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	dnsruntime "go.miragespace.co/specter/internal/dns"
	"go.miragespace.co/specter/internal/listen"
	acmeSpec "go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/util"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func Generate() *cli.Command {
	ip := util.GetOutboundIP()
	return &cli.Command{
		Name:        "dns",
		Usage:       "start acme dns on the edge",
		Description: `Handle ACME DNS challenge`,
		ArgsUsage:   " ",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:    "listen-addr",
				Aliases: []string{"listen"},
				Value:   []string{fmt.Sprintf("%s:53", ip.String())},
				Usage:   `Repeatable address:port to listen for incoming acme dns queries. Each entry serves both TCP and UDP unless overridden.`,
				Sources: cli.EnvVars("LISTEN_ADDR"),
			},
			&cli.StringSliceFlag{
				Name:        "listen-tcp",
				DefaultText: "same as listen-addr",
				Usage:       "Override the listen address and port for TCP (repeatable)",
				Sources:     cli.EnvVars("LISTEN_TCP"),
			},
			&cli.StringSliceFlag{
				Name:        "listen-udp",
				DefaultText: "same as listen-addr",
				Usage:       "Override the listen address and port for UDP (repeatable). Required if environment needs a specific address, such as on fly.io",
				Sources:     cli.EnvVars("LISTEN_UDP"),
			},
			&cli.StringFlag{
				Name:     "rpc",
				Value:    "tcp://127.0.0.1:11180",
				Required: true,
				Usage:    `Specter server's listen-rpc endpoint. This is required to lookup acme challenge on the chord network.`,
			},
			&cli.StringFlag{
				Name:        "acme",
				DefaultText: "acme://{ACME_EMAIL}:@acmehostedzone.com",
				Required:    true,
				Sources:     cli.EnvVars("ACME_URI"),
				Usage: `To enable acme dns, provide an email for the issuer, and the delegated zone for hosting challenges.
			Alternatively, you can set the URI via the environment variable ACME_URI.`,
			},
			&cli.StringSliceFlag{
				Name:     "acme-ns",
				Sources:  cli.EnvVars("ACME_NS"),
				Required: true,
				Usage: `If acme dns is enabled, specify the delegated zone's A/AAAA records. For example, ns1.acmehostedzone.com/93.184.216.34.
			This is needed to delegate acme dns challenges to specter.
			Multiple records can be separated with a comma.`,
			},

			// used for acme setup internally
			&cli.StringFlag{
				Name:   "acme_email",
				Hidden: true,
			},
			&cli.StringFlag{
				Name:   "acme_zone",
				Hidden: true,
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			email, zone, err := acmeSpec.ParseAcmeURI(cmd.String("acme"))
			if err != nil {
				return ctx, err
			}

			ns := make(map[string][]string)
			records := cmd.StringSlice("acme-ns")
			for _, r := range records {
				parts := strings.Split(r, "/")
				if len(parts) != 2 {
					return ctx, fmt.Errorf("unable to parse record: %s", r)
				}
				domain := parts[0]
				_, ok := ns[domain]
				if !ok {
					ns[domain] = make([]string, 0)
				}
				ns[domain] = append(ns[domain], parts[1])
			}
			cmd.Root().Metadata["ns"] = ns

			cmd.Set("acme_email", email)
			cmd.Set("acme_zone", zone)

			return ctx, nil
		},
		Action: cmdDNS,
	}
}

func cmdDNS(ctx context.Context, cmd *cli.Command) error {
	logger, ok := cmd.Root().Metadata["logger"].(*zap.Logger)
	if !ok || logger == nil {
		return fmt.Errorf("unable to obtain logger from app context")
	}

	listenBase := cmd.StringSlice("listen-addr")
	tcpAddrs, err := listen.ParseAddresses("tcp",
		listenBase,
		cmd.StringSlice("listen-tcp"),
	)
	if err != nil {
		return fmt.Errorf("error parsing tcp listen address: %w", err)
	}

	udpAddrs, err := listen.ParseAddresses("udp",
		listenBase,
		cmd.StringSlice("listen-udp"),
	)
	if err != nil {
		return fmt.Errorf("error parsing udp listen address: %w", err)
	}

	if len(listenBase) == 0 {
		return fmt.Errorf("at least one listen-addr must be provided")
	}

	var (
		dialNetwork string
		dialAddress string
	)
	parsedRpc, err := url.Parse(cmd.String("rpc"))
	if err != nil {
		return fmt.Errorf("error parsing rpc address: %w", err)
	}
	switch parsedRpc.Scheme {
	case "unix":
		dialNetwork = "unix"
		dialAddress = parsedRpc.Path
	case "tcp":
		dialNetwork = "tcp"
		dialAddress = parsedRpc.Host
	default:
		return fmt.Errorf("unknown scheme for rpc address: %s", parsedRpc.Scheme)
	}

	return dnsruntime.Run(ctx, dnsruntime.Options{
		Logger:          logger,
		Version:         cmd.Root().Version,
		TCPAddresses:    tcpAddrs,
		UDPAddresses:    udpAddrs,
		RPCNetwork:      dialNetwork,
		RPCAddress:      dialAddress,
		ACMEEmail:       cmd.String("acme_email"),
		ACMEZone:        cmd.String("acme_zone"),
		ACMENameservers: cmd.Root().Metadata["ns"].(map[string][]string),
	})
}
