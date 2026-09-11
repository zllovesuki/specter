package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"go.miragespace.co/specter/acme"
	chordImpl "go.miragespace.co/specter/chord"
	cmdlisten "go.miragespace.co/specter/cmd/internal/listen"
	"go.miragespace.co/specter/gateway"
	"go.miragespace.co/specter/overlay"
	"go.miragespace.co/specter/pki"
	"go.miragespace.co/specter/rtt"
	acmeSpec "go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/cipher"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/transport/q"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/timing"
	"go.miragespace.co/specter/tun/server"
	"go.miragespace.co/specter/util"
	"go.miragespace.co/specter/util/migrator"
	"go.miragespace.co/specter/util/reuse"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/alecthomas/units"
	"github.com/getsentry/sentry-go"
	"github.com/pires/go-proxyproto"
	"github.com/quic-go/quic-go"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

type certBundle struct {
	ca           *x509.CertPool
	clientCa     *x509.CertPool
	clientCaCert tls.Certificate
	node         tls.Certificate
}

func certLoaderFilesystem(dir string) (*certBundle, error) {
	files := []string{"ca.crt", "client-ca.crt", "client-ca.key", "node.crt", "node.key"}
	for i, name := range files {
		files[i] = filepath.Join(dir, name)
	}
	caCert, err := os.ReadFile(files[0])
	if err != nil {
		return nil, fmt.Errorf("reading ca cert from file: %w", err)
	}
	clientCaCert, err := os.ReadFile(files[1])
	if err != nil {
		return nil, fmt.Errorf("reading client ca cert from file: %w", err)
	}
	clientCaKey, err := os.ReadFile(files[2])
	if err != nil {
		return nil, fmt.Errorf("reading client ca key from file: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("unable to use provided ca bundle")
	}
	clientCaCertPool := x509.NewCertPool()
	if ok := clientCaCertPool.AppendCertsFromPEM(clientCaCert); !ok {
		return nil, fmt.Errorf("unable to use provided client ca bundle")
	}
	node, err := tls.LoadX509KeyPair(files[3], files[4])
	if err != nil {
		return nil, fmt.Errorf("creating node cert/key from files: %w", err)
	}
	clientCa, err := tls.X509KeyPair(clientCaCert, clientCaKey)
	if err != nil {
		return nil, fmt.Errorf("creating client ca cert/key from files: %w", err)
	}
	return &certBundle{
		ca:           caCertPool,
		clientCa:     clientCaCertPool,
		clientCaCert: clientCa,
		node:         node,
	}, nil
}

func certLoaderEnv(cmd *cli.Command) (*certBundle, error) {
	caCert, err := base64.StdEncoding.DecodeString(cmd.String("env_ca"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_CA: %w", err)
	}
	nodeCert, err := base64.StdEncoding.DecodeString(cmd.String("env_node"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE: %w", err)
	}
	nodeKey, err := base64.StdEncoding.DecodeString(cmd.String("env_node_key"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE_KEY: %w", err)
	}
	clientCaCert, err := base64.StdEncoding.DecodeString(cmd.String("env_client_ca"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE: %w", err)
	}
	clientCaKey, err := base64.StdEncoding.DecodeString(cmd.String("env_client_ca_key"))
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode CERT_NODE_KEY: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("unable to use provided ca bundle")
	}
	clientCaCertPool := x509.NewCertPool()
	if ok := clientCaCertPool.AppendCertsFromPEM(clientCaCert); !ok {
		return nil, fmt.Errorf("unable to use provided client ca bundle")
	}
	node, err := tls.X509KeyPair(nodeCert, nodeKey)
	if err != nil {
		return nil, fmt.Errorf("creating node cert/key from env: %w", err)
	}
	clientCa, err := tls.X509KeyPair(clientCaCert, clientCaKey)
	if err != nil {
		return nil, fmt.Errorf("creating client ca cert/key from env: %w", err)
	}
	return &certBundle{
		ca:           caCertPool,
		clientCa:     clientCaCertPool,
		clientCaCert: clientCa,
		node:         node,
	}, nil
}

func configCertProvider(cmd *cli.Command, logger *zap.Logger, kv chord.VNode) (cipher.CertProvider, error) {
	rootDomains := cmd.StringSlice("apex")
	managedDomains := rootDomains

	if cmd.IsSet("acme") {
		acmeSolver := &acme.ChordSolver{
			KV:             kv,
			ManagedDomains: managedDomains,
		}
		manager, err := acme.NewManager(acme.ManagerConfig{
			Logger:         logger,
			KV:             kv,
			DNSSolver:      acmeSolver,
			ManagedDomains: managedDomains,
			CA:             cmd.String("acme_ca"),
			Email:          cmd.String("acme_email"),
		})
		if err != nil {
			return nil, err
		}
		logger.Info("Using acme as cert provider", zap.String("email", cmd.String("acme_email")), zap.String("zone", cmd.String("acme_zone")))
		return manager, nil
	} else {
		logger.Info("Using self-signed as cert provider")
		self := &SelfSignedProvider{
			RootDomain: rootDomains[0],
		}
		return self, nil
	}
}

func modifyToSentryLogger(logger *zap.Logger, client *sentry.Client) *zap.Logger {
	cfg := zapsentry.Configuration{
		Level:             zapcore.WarnLevel,
		EnableBreadcrumbs: true,
		BreadcrumbLevel:   zapcore.InfoLevel,
	}
	core, err := zapsentry.NewCore(cfg, zapsentry.NewSentryClientFromClient(client))

	if err != nil {
		logger.Warn("failed to init zap", zap.Error(err))
	}

	logger = zapsentry.AttachCoreToLogger(core, logger)

	return logger
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

	dataDir, err := filepath.Abs(cmd.String("data-dir"))
	if err != nil {
		return fmt.Errorf("resolving data directory: %w", err)
	}
	kvOption := cmd.String("kv-provider")
	if err := preflightKVProvider(kvOption, dataDir); err != nil {
		return fmt.Errorf("storage preflight: %w", err)
	}
	logger.Info("Storage configuration", zap.String("provider", kvOption), zap.String("data_dir", dataDir))

	var gwOptions gateway.Options
	proxyBuffer, err := units.ParseStrictBytes(cmd.String("proxy-buffer"))
	if err != nil {
		return fmt.Errorf("error parsing proxy buffer size: %w", err)
	}
	gwOptions.ProxyBufferSize = int(proxyBuffer)
	transportBuffer, err := units.ParseStrictBytes(cmd.String("transport-buffer"))
	if err != nil {
		return fmt.Errorf("error parsing transport buffer size: %w", err)
	}
	gwOptions.TransportBufferSize = int(transportBuffer)

	if cmd.IsSet("sentry") {
		client, err := sentry.NewClient(sentry.ClientOptions{
			Dsn:     cmd.String("sentry"),
			Release: cmd.Root().Version,
		})
		if err != nil {
			return fmt.Errorf("initializing sentry client: %w", err)
		}
		defer client.Flush(time.Second * 2)

		logger = modifyToSentryLogger(logger, client)
		defer logger.Sync()
	}

	listenBase := cmd.StringSlice("listen-addr")
	tcpAddrs, err := cmdlisten.ParseAddresses("tcp",
		listenBase,
		cmd.StringSlice("listen-tcp"),
	)
	if err != nil {
		return fmt.Errorf("error parsing tcp listen address: %w", err)
	}

	udpAddrs, err := cmdlisten.ParseAddresses("udp",
		listenBase,
		cmd.StringSlice("listen-udp"),
	)
	if err != nil {
		return fmt.Errorf("error parsing udp listen address: %w", err)
	}

	addrStrings := func(addrs []cmdlisten.Address) []string {
		out := make([]string, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, a.Address)
		}
		return out
	}

	logger.Info("listener configuration",
		zap.Strings("tcp", addrStrings(tcpAddrs)),
		zap.Strings("udp", addrStrings(udpAddrs)),
	)

	if len(listenBase) == 0 {
		return fmt.Errorf("at least one listen-addr must be provided")
	}

	advertise := listenBase[0]

	if cmd.IsSet("advertise-addr") {
		advertise = cmd.String("advertise-addr")
	}
	_, advertisePortStr, err := net.SplitHostPort(advertise)
	if err != nil {
		return fmt.Errorf("error parsing advertise address: %w", err)
	}
	advertisePort, err := strconv.ParseInt(advertisePortStr, 10, 32)
	if err != nil {
		return fmt.Errorf("error parsing advertise port: %w", err)
	}

	var bundle *certBundle
	if cmd.IsSet("cert-dir") {
		bundle, err = certLoaderFilesystem(cmd.String("cert-dir"))
		if err != nil {
			return fmt.Errorf("error loading certificates from directory: %w", err)
		}
	} else if cmd.IsSet("cert-env") {
		bundle, err = certLoaderEnv(cmd)
		if err != nil {
			return fmt.Errorf("error loading certificates from environment variable: %w", err)
		}
	}

	listenCfg := &net.ListenConfig{
		Control: reuse.Control,
	}

	var (
		rpcListener net.Listener
	)
	if cmd.IsSet("listen-rpc") {
		parsedRpc, err := url.Parse(cmd.String("listen-rpc"))
		if err != nil {
			return fmt.Errorf("error parsing rpc listen address: %w", err)
		}
		switch parsedRpc.Scheme {
		case "unix":
			rpcListener, err = listenCfg.Listen(ctx, "unix", parsedRpc.Path)
		case "tcp":
			rpcListener, err = listenCfg.Listen(ctx, "tcp", parsedRpc.Host)
		default:
			return fmt.Errorf("unknown scheme for rpc listen address: %s", parsedRpc.Scheme)
		}
		if err != nil {
			return fmt.Errorf("error setting up rpc listener: %w", err)
		}
		defer rpcListener.Close()
	}

	tcpListeners := make([]net.Listener, 0, len(tcpAddrs))
	for _, addr := range tcpAddrs {
		l, err := listenCfg.Listen(ctx, addr.Network, addr.Address)
		if err != nil {
			return fmt.Errorf("error setting up gateway tcp listener on %s: %w", addr.Address, err)
		}
		if cmd.Bool("proxy-protocol") {
			l = &proxyproto.Listener{
				Listener:          l,
				ReadHeaderTimeout: time.Second * 3,
				Policy: func(upstream net.Addr) (proxyproto.Policy, error) {
					return proxyproto.REQUIRE, nil
				},
			}
		}
		tcpListeners = append(tcpListeners, l)
	}
	if len(tcpListeners) == 0 {
		return fmt.Errorf("no tcp listeners configured")
	}

	// TODO: implement SNI proxy so specter can share port with another webserver
	tcpListener := tcpListeners[0]
	if len(tcpListeners) > 1 {
		tcpListener = newMultiListener(tcpListeners)
	}
	defer tcpListener.Close()

	udpBindings := make([]udpBinding, 0, len(udpAddrs))
	for _, addr := range udpAddrs {
		pconn, err := listenCfg.ListenPacket(ctx, addr.Network, addr.Address)
		if err != nil {
			return fmt.Errorf("error setting up gateway udp listener on %s: %w", addr.Address, err)
		}
		tr := &quic.Transport{Conn: pconn}
		udpBindings = append(udpBindings, udpBinding{
			listen:     addr,
			packetConn: pconn,
			transport:  tr,
		})
	}
	if len(udpBindings) == 0 {
		return fmt.Errorf("no udp listeners configured")
	}
	for i := range udpBindings {
		defer udpBindings[i].transport.Close()
		defer udpBindings[i].packetConn.Close()
	}

	var httpListener net.Listener
	if advertisePort == 443 || cmd.IsSet("listen-http") {
		httpListeners := make([]net.Listener, 0, len(tcpAddrs))
		seenHost := make(map[string]struct{}, len(tcpAddrs))
		for _, addr := range tcpAddrs {
			if _, ok := seenHost[addr.Host]; ok {
				continue
			}
			seenHost[addr.Host] = struct{}{}
			httpAddr := net.JoinHostPort(addr.Host, strconv.Itoa(cmd.Int("listen-http")))
			l, err := listenCfg.Listen(ctx, cmdlisten.NetworkForVersion("tcp", addr.Version), httpAddr)
			if err != nil {
				return fmt.Errorf("error setting up http listener on %s: %w", httpAddr, err)
			}
			if cmd.Bool("proxy-protocol") {
				l = &proxyproto.Listener{
					Listener:          l,
					ReadHeaderTimeout: time.Second * 3,
					Policy: func(upstream net.Addr) (proxyproto.Policy, error) {
						return proxyproto.REQUIRE, nil
					},
				}
			}
			httpListeners = append(httpListeners, l)
		}

		if len(httpListeners) > 0 {
			httpListener = httpListeners[0]
			if len(httpListeners) > 1 {
				httpListener = newMultiListener(httpListeners)
			}
			defer httpListener.Close()
		}
	}

	alpnMuxes := make([]*overlay.ALPNMux, 0, len(udpBindings))
	for _, binding := range udpBindings {
		mux, err := overlay.NewMux(binding.transport)
		if err != nil {
			return fmt.Errorf("error setting up quic alpn muxer for %s: %w", binding.listen.Address, err)
		}
		alpnMuxes = append(alpnMuxes, mux)
		defer mux.Close()
	}

	chordName := fmt.Sprintf("chord://%s", advertise)
	tunnelName := fmt.Sprintf("tunnel://%s", advertise)

	logger.Info("Using advertise addresses as destinations", zap.String("chord", chordName), zap.String("tunnel", tunnelName))

	chordTLS := cipher.GetPeerTLSConfig(bundle.ca, bundle.node, []string{
		tun.ALPN(protocol.Link_SPECTER_CHORD),
	})

	// handles specter-chord/1
	chordListeners := make([]q.Listener, 0, len(alpnMuxes))
	for _, mux := range alpnMuxes {
		chordListeners = append(chordListeners, mux.With(chordTLS, tun.ALPN(protocol.Link_SPECTER_CHORD)))
	}
	chordListener := newMultiQuicListener(ctx, chordListeners)
	defer chordListener.Close()

	chordRTT := rtt.NewInstrumentation(20)
	dialer := newMultiDialer(udpBindings)
	chordTransport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:           logger.With(zapsentry.NewScope()).With(zap.String("component", "chordTransport")),
		VirtualTransport: true,
		ClientTLS:        chordTLS,
		RTTRecorder:      chordRTT,
		QuicTransport:    dialer,
		Endpoint: &protocol.Node{
			Address: advertise,
		},
	})
	defer chordTransport.Stop()

	// TODO: measure rtt to client to build routing table with cost
	tunnelTransport := overlay.NewQUIC(overlay.TransportConfig{
		UseCertificateIdentity: true,
		Logger:                 logger.With(zapsentry.NewScope()).With(zap.String("component", "tunnelTransport")),
		Endpoint: &protocol.Node{
			Address: advertise,
		},
		QuicTransport: dialer,
	})
	defer tunnelTransport.Stop()

	var existingNode chord.VNode
	chordClient := rpc.DynamicChordClient(ctx, chordTransport)
	if cmd.IsSet("join") {
		existingNode, err = chordImpl.NewRemoteNode(ctx, logger, chordClient, &protocol.Node{
			Unknown: true,
			Address: cmd.String("join"),
		})
		if err != nil {
			return fmt.Errorf("error connecting existing chord node: %w", err)
		}
	}

	streamRouter := transport.NewStreamRouter(logger.With(zapsentry.NewScope()).With(zap.String("component", "router")), chordTransport, tunnelTransport)
	virtualNodes := make([]*chordImpl.LocalNode, 0)

	k := cmd.Int("virtual")
	cacheDir := filepath.Join(dataDir, "cache")
	for i := range k {
		nodeIdentity := &protocol.Node{
			Id:      chord.Hash(fmt.Appendf(nil, "%s/%d", chordName, i)),
			Address: advertise,
		}
		kvProvider, stopFn, err := getKVProvider(
			logger.With(zapsentry.NewScope()).With(zap.String("component", "kv"), zap.Object("node", nodeIdentity)),
			kvOption,
			filepath.Join(dataDir, fmt.Sprintf("%d", i)),
			cacheDir,
		)
		if err != nil {
			return fmt.Errorf("initializing kv storage: %w", err)
		}
		defer stopFn()

		virtualNode := chordImpl.NewLocalNode(chordImpl.NodeConfig{
			Identity:                 nodeIdentity,
			BaseLogger:               logger,
			ChordClient:              chordClient,
			KVProvider:               kvProvider,
			StabilizeInterval:        timing.ChordStabilizeInterval,
			FixFingerInterval:        timing.ChordFixFingerInterval,
			PredecessorCheckInterval: timing.ChordPredecessorCheckInterval,
			NodesRTT:                 chordRTT,
		})

		virtualNode.AttachRouter(ctx, streamRouter)
		virtualNodes = append(virtualNodes, virtualNode)
	}

	rootNode := virtualNodes[0]
	rootNode.AttachRoot(ctx, streamRouter)
	if rpcListener != nil {
		logger.Info("Exposing RPC externally", zap.String("listen", cmd.String("listen-rpc")))
		rootNode.AttachExternal(ctx, rpcListener)
	}

	go chordTransport.AcceptWithListener(ctx, chordListener)
	go streamRouter.Accept(ctx)
	for _, mux := range alpnMuxes {
		go mux.Accept(ctx)
	}

	if !cmd.IsSet("join") {
		if err := rootNode.Create(); err != nil {
			return fmt.Errorf("error bootstrapping chord ring: %w", err)
		}
	} else {
		if err := rootNode.Join(existingNode); err != nil {
			return fmt.Errorf("error joining root node to existing chord ring: %w", err)
		}
	}
	defer rootNode.Leave()

	for i := 1; i < k; i++ {
		p, err := chordImpl.NewRemoteNode(ctx, logger, chordClient, rootNode.Identity())
		if err != nil {
			return fmt.Errorf("error connecting to root node: %w", err)
		}
		if err := virtualNodes[i].Join(p); err != nil {
			return fmt.Errorf("error joining virtual node to root node: %w", err)
		}
		defer virtualNodes[i].Leave()
	}

	certProvider, err := configCertProvider(cmd, logger.With(zapsentry.NewScope()), chord.WrapRetryKV(rootNode, timing.ChordStabilizeInterval/2, 5))
	if err != nil {
		return fmt.Errorf("failed to configure cert provider: %w", err)
	}

	if err := certProvider.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize cert provider: %w", err)
	}

	gwTLSConf := cipher.GetGatewayTLSConfig(certProvider.GetCertificate, []string{
		tun.ALPN(protocol.Link_HTTP2),
		tun.ALPN(protocol.Link_HTTP),
		tun.ALPN(protocol.Link_TCP),
		tun.ALPN(protocol.Link_UNKNOWN),
	})

	gwH2Listener := tls.NewListener(tcpListener, gwTLSConf)
	defer gwH2Listener.Close()

	// handles h3, h3-29, and specter-tcp/1
	gwH3Listeners := make([]q.Listener, 0, len(alpnMuxes))
	for _, mux := range alpnMuxes {
		gwH3Listeners = append(gwH3Listeners, mux.With(gwTLSConf, append([]string{tun.ALPN(protocol.Link_TCP)}, cipher.H3Protos...)...))
	}
	gwH3Listener := newMultiQuicListener(ctx, gwH3Listeners)
	defer gwH3Listener.Close()

	// handles specter-client/1
	clientTLSConf := cipher.GetClientTLSConfig(bundle.clientCa, certProvider.GetCertificate, []string{tun.ALPN(protocol.Link_SPECTER_CLIENT)})
	clientListeners := make([]q.Listener, 0, len(alpnMuxes))
	for _, mux := range alpnMuxes {
		clientListeners = append(clientListeners, mux.With(clientTLSConf, tun.ALPN(protocol.Link_SPECTER_CLIENT)))
	}
	clientListener := newMultiQuicListener(ctx, clientListeners)
	defer clientListener.Close()

	tunnelIdentity := &protocol.Node{
		Id:      chord.Hash([]byte(tunnelName)),
		Address: advertise,
	}
	tunServer := server.New(server.Config{
		Logger:          logger.With(zapsentry.NewScope()).With(zap.String("component", "tunnelServer"), zap.Uint64("node", tunnelIdentity.GetId())),
		ParentContext:   ctx,
		Chord:           chord.WrapRetryKV(rootNode, timing.ChordStabilizeInterval/2, 5),
		TunnelTransport: tunnelTransport,
		ChordTransport:  chordTransport,
		Resolver:        net.DefaultResolver,
		CertProvider:    certProvider,
		Apex:            rootDomains[0],
		Acme:            cmd.String("acme_zone"),
	})
	defer tunServer.Stop()

	tunServer.AttachRouter(ctx, streamRouter)
	tunServer.MustRegister(ctx)

	go tunnelTransport.AcceptWithListener(ctx, clientListener)

	var acmeHandler http.Handler
	if mgr, ok := certProvider.(*acme.Manager); ok {
		acmeHandler = acme.AcmeManagerHandler(mgr)
	}
	gw := gateway.New(gateway.GatewayConfig{
		PKIServer: &pki.Server{
			Logger:   logger.With(zapsentry.NewScope()).With(zap.String("component", "pki")),
			ClientCA: bundle.clientCaCert,
		},
		Handlers: gateway.InternalHandlers{
			Acme:         acmeHandler,
			Chord:        chordImpl.ChordStatsHandler(rootNode, virtualNodes),
			Overview:     chordImpl.OverviewHandler(rootNode, virtualNodes, kvOption),
			TunnelServer: server.TunnelServerHandler(tunServer),
			Migrator:     migrator.ConfigMigratorHandler(logger.With(zapsentry.NewScope()).With(zap.String("component", "migrator")), bundle.clientCaCert),
		},
		Logger:            logger.With(zapsentry.NewScope()).With(zap.String("component", "gateway")),
		TunnelServer:      tunServer,
		HTTPListener:      httpListener,
		H2Listener:        gwH2Listener,
		H3Listener:        gwH3Listener,
		RootDomains:       managedDomains,
		GatewayPort:       int(advertisePort),
		Options:           gwOptions,
		AdminUser:         cmd.String("auth_user"),
		AdminPass:         cmd.String("auth_pass"),
		HandshakeHintFunc: tunServer.RoutesPreload,
	})
	defer gw.Close()

	gw.AttachRouter(ctx, streamRouter)
	gw.MustStart(ctx)

	certProvider.OnHandshake(gw.HandshakeEarlyHint)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigs:
		logger.Info("received signal to stop", zap.String("signal", sig.String()))
	case <-ctx.Done():
		logger.Info("context done", zap.Error(ctx.Err()))
	}

	return nil
}
