package dns

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"go.miragespace.co/specter/acme"
	acmeSpec "go.miragespace.co/specter/spec/acme"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/util"
	"go.miragespace.co/specter/util/reuse"

	"github.com/miekg/dns"
	"github.com/urfave/cli/v2"
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
			&cli.StringFlag{
				Name:    "listen-addr",
				Aliases: []string{"listen"},
				Value:   fmt.Sprintf("%s:53", ip.String()),
				Usage:   `Address and port to listen for incoming acme dns queries. This port will serve both TCP and UDP (unless overridden).`,
			},
			&cli.StringFlag{
				Name:        "listen-tcp",
				DefaultText: "same as listen-addr",
				Usage:       "Override the listen address and port for TCP",
			},
			&cli.StringFlag{
				Name:        "listen-udp",
				DefaultText: "same as listen-addr",
				Usage:       "Override the listen address and port for UDP. Required if environment needs a specific address, such as on fly.io",
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
				EnvVars:     []string{"ACME_URI"},
				Usage: `To enable acme dns, provide an email for the issuer, and the delegated zone for hosting challenges.
			Alternatively, you can set the URI via the environment variable ACME_URI.`,
			},
			&cli.StringSliceFlag{
				Name:     "acme-ns",
				EnvVars:  []string{"ACME_NS"},
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
		Before: func(ctx *cli.Context) error {
			email, zone, err := acmeSpec.ParseAcmeURI(ctx.String("acme"))
			if err != nil {
				return err
			}

			ns := make(map[string][]string)
			records := ctx.StringSlice("acme-ns")
			for _, r := range records {
				parts := strings.Split(r, "/")
				if len(parts) != 2 {
					return fmt.Errorf("unable to parse record: %s", r)
				}
				domain := parts[0]
				_, ok := ns[domain]
				if !ok {
					ns[domain] = make([]string, 0)
				}
				ns[domain] = append(ns[domain], parts[1])
			}

			ctx.App.Metadata["ns"] = ns

			ctx.Set("acme_email", email)
			ctx.Set("acme_zone", zone)

			return nil
		},
		Action: cmdDNS,
	}
}

func cmdDNS(ctx *cli.Context) error {
	logger, ok := ctx.App.Metadata["logger"].(*zap.Logger)
	if !ok || logger == nil {
		return fmt.Errorf("unable to obtain logger from app context")
	}

	var (
		listenTcp = ctx.String("listen-addr")
		listenUdp = ctx.String("listen-addr")
		err       error
	)

	if ctx.IsSet("listen-tcp") {
		listenTcp = ctx.String("listen-tcp")
	}
	_, _, err = net.SplitHostPort(listenTcp)
	if err != nil {
		return fmt.Errorf("error parsing tcp listen address: %w", err)
	}

	if ctx.IsSet("listen-udp") {
		listenUdp = ctx.String("listen-udp")
	}
	_, _, err = net.SplitHostPort(listenUdp)
	if err != nil {
		return fmt.Errorf("error parsing udp listen address: %w", err)
	}

	var (
		dialNetwork string
		dialAddress string
	)
	parsedRpc, err := url.Parse(ctx.String("rpc"))
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

	listenCfg := &net.ListenConfig{
		Control: reuse.Control,
	}

	dnsTcpListener, err := listenCfg.Listen(ctx.Context, "tcp", listenTcp)
	if err != nil {
		return fmt.Errorf("error setting up dns tcp listener: %w", err)
	}
	defer dnsTcpListener.Close()

	dnsUdpListener, err := listenCfg.ListenPacket(ctx.Context, "udp", listenUdp)
	if err != nil {
		return fmt.Errorf("error setting up dns udp listener: %w", err)
	}
	defer dnsUdpListener.Close()

	dialer := &net.Dialer{}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxConnsPerHost = 100
	t.MaxIdleConnsPerHost = 5
	t.DisableCompression = true
	t.IdleConnTimeout = time.Minute
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, dialNetwork, dialAddress)
	}
	client := protocol.NewKVServiceProtobufClient("http://rpc", &http.Client{
		Transport: t,
	})
	kv := &RemoteKV{
		Client: client,
	}

	acmeDomain := ctx.String("acme_zone")
	acmeDNS := acme.NewDNS(
		ctx.Context,
		logger.With(zap.String("component", "acme_dns")),
		kv,
		ctx.String("acme_email"),
		acmeDomain,
		ctx.App.Metadata["ns"].(map[string][]string),
	)

	dnsMux := dns.NewServeMux()
	dnsMux.Handle(acmeDomain, acmeDNS)
	dnsMux.Handle(".", dns.HandlerFunc(Chaos(ctx.App.Version)))

	dnsTcpServer := &dns.Server{
		Listener: dnsTcpListener,
		Handler:  dnsMux,
		NotifyStartedFunc: func() {
			logger.Info("ACME DNS started", zap.String("proto", "tcp"), zap.String("listen", listenTcp))
		},
	}

	pconn := dnsUdpListener
	if runtime.GOOS == "illumos" {
		// needed to force net.PacketConn path instead of *net.UDPConn path
		// because of dual stack not working on illumos
		pconn = &squashed{PacketConn: dnsUdpListener}
	}
	dnsUdpServer := &dns.Server{
		PacketConn: pconn,
		Handler:    dnsMux,
		NotifyStartedFunc: func() {
			logger.Info("ACME DNS started", zap.String("proto", "udp"), zap.String("listen", listenUdp))
		},
	}

	go dnsTcpServer.ActivateAndServe()
	go dnsUdpServer.ActivateAndServe()

	defer dnsTcpServer.Shutdown()
	defer dnsUdpServer.Shutdown()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigs:
		logger.Info("received signal to stop", zap.String("signal", sig.String()))
	case <-ctx.Context.Done():
		logger.Info("context done", zap.Error(ctx.Context.Err()))
	}

	return nil
}

type RemoteKV struct {
	chord.KV
	Client protocol.KVService
}

func (r *RemoteKV) PrefixList(ctx context.Context, prefix []byte) (children [][]byte, err error) {
	resp, err := r.Client.List(ctx, &protocol.PrefixRequest{
		Prefix: prefix,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetChildren(), nil
}

func Chaos(version string) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)

		q := r.Question[0]
		if q.Name == "version.bind." && q.Qtype == dns.TypeTXT && q.Qclass == dns.ClassCHAOS {
			m.Answer = []dns.RR{
				&dns.TXT{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeTXT,
						Class:  dns.ClassCHAOS,
						Ttl:    3600,
					},
					Txt: []string{version},
				},
			}
		} else {
			m.MsgHdr.Rcode = dns.RcodeRefused
		}

		w.WriteMsg(m)
	}
}

type squashed struct {
	net.PacketConn
}
