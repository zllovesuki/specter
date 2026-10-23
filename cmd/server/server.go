package server

import (
	"context"
	"fmt"

	serverruntime "go.miragespace.co/specter/internal/server"
	acmeSpec "go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/cipher"
	"go.miragespace.co/specter/util"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func Generate() *cli.Command {
	ip := util.GetOutboundIP()
	return &cli.Command{
		Name:  "server",
		Usage: "start an specter server on the edge",
		Description: `Start a Specter server that joins a Chord DHT ring, routes all tunnels over QUIC, issues/renews TLS certificates via ACME DNS-01, and persists state for high availability.

	Specter server provides an internal endpoint on /_internal under apex domain. To enable internal endpoint, provide username and password 
	under environment variables INTERNAL_USER and INTERNAL_PASS. Absent of them will disable the internal endpoint entirely.

	Warning: do not use certificates issued by public CA for inter-node certificates, otherwise anyone can join your specter network`,
		ArgsUsage: " ",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "advertise-addr",
				Aliases:     []string{"advertise"},
				Sources:     cli.EnvVars("ADVERTISE_ADDR"),
				DefaultText: "same as listen-addr",
				Value:       fmt.Sprintf("%s:443", ip.String()),
				Usage: `Address and port to advertise to specter servers and clients to connect to.
			Note that specter will use advertised address to derive its Identity hash.`,
				Category: "Network Options",
			},
			&cli.StringSliceFlag{
				Name:    "listen-addr",
				Aliases: []string{"listen"},
				Value:   []string{fmt.Sprintf("%s:443", ip.String())},
				Usage: `Repeatable address:port to listen for specter server, specter client and gateway connections. Each entry serves both TCP and UDP unless overridden.
			Note that if specter is listening on port 443, it will also listen on port 80 to handle http connect proxy, and redirect other http requests to https`,
				Category: "Network Options",
				Sources:  cli.EnvVars("LISTEN_ADDR"),
			},
			&cli.StringSliceFlag{
				Name:        "listen-tcp",
				DefaultText: "same as listen-addr",
				Usage:       "Override the listen address and port for TCP (repeatable)",
				Category:    "Network Options",
				Sources:     cli.EnvVars("LISTEN_TCP"),
			},
			&cli.StringSliceFlag{
				Name:        "listen-udp",
				DefaultText: "same as listen-addr",
				Usage:       "Override the listen address and port for UDP (repeatable). Required if environment needs a specific address, such as on fly.io",
				Category:    "Network Options",
				Sources:     cli.EnvVars("LISTEN_UDP"),
			},
			&cli.BoolFlag{
				Name:     "proxy-protocol",
				Value:    false,
				Usage:    "Parse client IP via PROXY protocol (v1 or v2) when handling TCP connections. Required if environment is behind a TCP Load Balancer, such as on fly.io",
				Category: "Network Options",
			},

			&cli.StringFlag{
				Name:  "listen-rpc",
				Value: "tcp://127.0.0.1:11180",
				Usage: `Expose chord's RPC for VNode and KV to an external program. This is required to use with specter's acme dns.
			NOTE: The listener is exposed without any authentication or authorization. You should only expose it to localhost or unix socket`,
				Category: "Server Options",
			},
			&cli.StringFlag{
				Name:     "data-dir",
				Aliases:  []string{"data"},
				Usage:    "Path to directory that will be used for persisting non-volatile KV data",
				Required: true,
				Category: "Server Options",
			},
			&cli.StringFlag{
				Name:     "cert-dir",
				Aliases:  []string{"cert"},
				Usage:    `Path to directory containing ca.crt, client-ca.crt, client-ca.key, node.crt, and node.key for mutual TLS between specter server nodes`,
				Category: "Server Options",
			},
			&cli.BoolFlag{
				Name: "cert-env",
				Usage: `Load ca.crt (CERT_CA), client-ca.crt (CERT_CLIENT_CA), client-ca.key (CERT_CLIENT_CA_KEY), node.crt (CERT_NODE), and node.key (CERT_NODE_KEY) from environment variables encoded as base64.
			This can be set instead of loading from cert-dir. Required if environment prefers loading secrets from ENV, such as on fly.io`,
				Category: "Server Options",
			},
			&cli.StringFlag{
				Name:        "sentry",
				DefaultText: "https://public@sentry.example.com/1",
				Usage:       "Sentry DSN for error monitoring. Alternatively, you can set the DSN via the environment variable SENTRY_DSN",
				Sources:     cli.EnvVars("SENTRY_DSN"),
				Category:    "Server Options",
			},

			&cli.IntFlag{
				Name:     "virtual",
				Usage:    "Number of virtual nodes to be started as part of the chord ring",
				Value:    5,
				Category: "Chord Options",
			},
			&cli.StringFlag{
				Name:    "join",
				Sources: cli.EnvVars("CHORD_JOIN"),
				Usage: `A known specter server's advertise address.
			Absent of this flag will bootstrap a new cluster with current node as the seed node`,
				Category: "Chord Options",
			},
			&cli.StringFlag{
				Name:     "kv-provider",
				Usage:    "Backend storage provider for KV. Valid options are memory, aof, and sqlite",
				Value:    "aof",
				Sources:  cli.EnvVars("KV_PROVIDER"),
				Category: "Chord Options",
			},

			&cli.StringFlag{
				Name:        "acme",
				DefaultText: "acme://{ACME_EMAIL}:@acmehostedzone.com",
				Sources:     cli.EnvVars("ACME_URI"),
				Usage: `To enable acme, provide an email for the issuer, and the delegated zone for hosting challenges.
			Absent of this flag will serve self-signed certificate.
			Alternatively, you can set the URI via the environment variable ACME_URI.`,
				Category: "Gateway Options",
			},
			&cli.IntFlag{
				Name:  "listen-http",
				Value: 80,
				Usage: `Override the listening port of the http handler, which handles http connect proxy, and redirects other http requests to https.
			Note by default the http handler will not be started unless the node is advertising on port 443. Using this option will force the http handler to start.`,
				Category: "Gateway Options",
			},
			&cli.StringSliceFlag{
				Name:     "apex",
				Sources:  cli.EnvVars("APEX"),
				Usage:    "Canonical domain to be used as tunnel root domain. Tunnels will be given names under *.`APEX`. Additional canonical domains can be specified.",
				Required: true,
				Category: "Gateway Options",
			},
			&cli.StringFlag{
				Name:     "transport-buffer",
				Value:    "16KiB",
				Usage:    "Buffer size when making HTTP request to client",
				Category: "Gateway Options",
			},
			&cli.StringFlag{
				Name:     "proxy-buffer",
				Value:    "16KiB",
				Usage:    "Buffer size when copying response from client",
				Category: "Gateway Options",
			},

			&cli.BoolFlag{
				Name:     "print-acme",
				Value:    false,
				Usage:    "Print acme setup instructions for apex domains based on current configuration.",
				Category: "Miscellaneous",
			},

			&cli.StringFlag{
				Name:    "acme_ca",
				Hidden:  true,
				Value:   cipher.CertCA,
				Sources: cli.EnvVars("ACME_CA"),
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
			&cli.StringFlag{
				Name:    "auth_user",
				Hidden:  true,
				Sources: cli.EnvVars("INTERNAL_USER"),
			},
			&cli.StringFlag{
				Name:    "auth_pass",
				Hidden:  true,
				Sources: cli.EnvVars("INTERNAL_PASS"),
			},
			&cli.StringFlag{
				Name:    "env_ca",
				Hidden:  true,
				Sources: cli.EnvVars("CERT_CA"),
			},
			&cli.StringFlag{
				Name:    "env_node",
				Hidden:  true,
				Sources: cli.EnvVars("CERT_NODE"),
			},
			&cli.StringFlag{
				Name:    "env_node_key",
				Hidden:  true,
				Sources: cli.EnvVars("CERT_NODE_KEY"),
			},
			&cli.StringFlag{
				Name:    "env_client_ca",
				Hidden:  true,
				Sources: cli.EnvVars("CERT_CLIENT_CA"),
			},
			&cli.StringFlag{
				Name:    "env_client_ca_key",
				Hidden:  true,
				Sources: cli.EnvVars("CERT_CLIENT_CA_KEY"),
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			if !cmd.IsSet("cert-dir") && !cmd.IsSet("cert-env") {
				return ctx, fmt.Errorf("no certificate loader is specified")
			}
			if cmd.Int("virtual") < 1 {
				return ctx, fmt.Errorf("minimum of 1 virtual node is required")
			}
			if cmd.IsSet("acme") {
				email, zone, err := acmeSpec.ParseAcmeURI(cmd.String("acme"))
				if err != nil {
					return ctx, err
				}
				cmd.Set("acme_email", email)
				cmd.Set("acme_zone", zone)
			}
			return ctx, nil
		},
		Action: cmdServer,
	}
}

func cmdServer(ctx context.Context, cmd *cli.Command) error {
	logger, ok := cmd.Root().Metadata["logger"].(*zap.Logger)
	if !ok || logger == nil {
		return fmt.Errorf("unable to obtain logger from app context")
	}

	rootDomains := cmd.StringSlice("apex")
	managedDomains := rootDomains

	if cmd.Bool("print-acme") {
		if !cmd.IsSet("acme") {
			return fmt.Errorf("acme is not configured")
		}
		for _, d := range managedDomains {
			hostname, err := acmeSpec.Normalize(d)
			if err != nil {
				return fmt.Errorf("error normalizing domain for %s: %w", d, err)
			}
			name, content := acmeSpec.GenerateManagedRecord(hostname, cmd.String("acme_zone"))
			logger.Info("ACME DNS Record", zap.String("name", name), zap.String("content", content), zap.String("type", "CNAME"))
		}
		return nil
	}

	return serverruntime.Run(ctx, serverOptions(cmd, logger))
}

func serverOptions(cmd *cli.Command, logger *zap.Logger) serverruntime.Options {
	options := serverruntime.Options{
		Logger:           logger,
		Version:          cmd.Root().Version,
		DataDir:          cmd.String("data-dir"),
		KVProvider:       cmd.String("kv-provider"),
		ListenAddresses:  cmd.StringSlice("listen-addr"),
		ListenTCP:        cmd.StringSlice("listen-tcp"),
		ListenUDP:        cmd.StringSlice("listen-udp"),
		AdvertiseAddress: optionalString(cmd, "advertise-addr"),
		RPCListen:        optionalString(cmd, "listen-rpc"),
		ProxyProtocol:    cmd.Bool("proxy-protocol"),
		CertDirectory:    optionalString(cmd, "cert-dir"),
		VirtualNodes:     cmd.Int("virtual"),
		JoinAddress:      optionalString(cmd, "join"),
		SentryDSN:        optionalString(cmd, "sentry"),
		Apex:             cmd.StringSlice("apex"),
		ACMEEnabled:      cmd.IsSet("acme"),
		ACMECA:           cmd.String("acme_ca"),
		ACMEEmail:        cmd.String("acme_email"),
		ACMEZone:         cmd.String("acme_zone"),
		ProxyBuffer:      cmd.String("proxy-buffer"),
		TransportBuffer:  cmd.String("transport-buffer"),
		AdminUser:        cmd.String("auth_user"),
		AdminPass:        cmd.String("auth_pass"),
	}
	if cmd.IsSet("listen-http") {
		port := cmd.Int("listen-http")
		options.HTTPPort = &port
	}
	if cmd.IsSet("cert-env") {
		options.CertEnvironment = &serverruntime.CertificateEnvironment{
			CA:          cmd.String("env_ca"),
			Node:        cmd.String("env_node"),
			NodeKey:     cmd.String("env_node_key"),
			ClientCA:    cmd.String("env_client_ca"),
			ClientCAKey: cmd.String("env_client_ca_key"),
		}
	}
	return options
}

func optionalString(cmd *cli.Command, name string) *string {
	if !cmd.IsSet(name) {
		return nil
	}
	value := cmd.String(name)
	return &value
}
