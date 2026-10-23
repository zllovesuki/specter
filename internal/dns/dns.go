package dns

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"go.miragespace.co/specter/acme"
	"go.miragespace.co/specter/internal/listen"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/util/reuse"

	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type Options struct {
	Logger          *zap.Logger
	Version         string
	TCPAddresses    []listen.Address
	UDPAddresses    []listen.Address
	RPCNetwork      string
	RPCAddress      string
	ACMEEmail       string
	ACMEZone        string
	ACMENameservers map[string][]string
}

// Run serves the ACME challenge zone until cancellation or a termination signal.
func Run(ctx context.Context, options Options) error {
	logger := options.Logger

	addrStrings := func(addrs []listen.Address) []string {
		out := make([]string, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, a.Address)
		}
		return out
	}
	logger.Info("acme dns listener configuration",
		zap.Strings("tcp", addrStrings(options.TCPAddresses)),
		zap.Strings("udp", addrStrings(options.UDPAddresses)),
	)

	listenCfg := &net.ListenConfig{
		Control: reuse.Control,
	}

	dialer := &net.Dialer{}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxConnsPerHost = 100
	t.MaxIdleConnsPerHost = 5
	t.DisableCompression = true
	t.IdleConnTimeout = time.Minute
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, options.RPCNetwork, options.RPCAddress)
	}
	httpClient := &http.Client{
		Transport: t,
	}
	defer t.CloseIdleConnections()
	client := protocol.NewKVServiceProtobufClient("http://rpc", httpClient)
	kv := &remoteKV{
		Client: client,
	}

	acmeDomain := options.ACMEZone
	acmeDNS := acme.NewDNS(ctx, logger.With(zap.String("component", "acme_dns")),
		kv,
		options.ACMEEmail,
		acmeDomain, options.ACMENameservers,
	)

	dnsMux := dns.NewServeMux()
	dnsMux.Handle(acmeDomain, acmeDNS)
	dnsMux.Handle(".", chaos(options.Version))

	var tcpServers, udpServers []*dns.Server
	defer func() {
		for _, srv := range tcpServers {
			srv.Shutdown()
			srv.Listener.Close()
		}
		for _, srv := range udpServers {
			srv.Shutdown()
			srv.PacketConn.Close()
		}
	}()

	for _, addr := range options.TCPAddresses {
		ln, err := listenCfg.Listen(ctx, addr.Network, addr.Address)
		if err != nil {
			return fmt.Errorf("error setting up dns tcp listener on %s: %w", addr.Address, err)
		}
		listenAddr := addr.Address
		server := &dns.Server{
			Listener: ln,
			Handler:  dnsMux,
			NotifyStartedFunc: func() {
				logger.Info("ACME DNS started", zap.String("proto", "tcp"), zap.String("listen", listenAddr))
			},
		}
		tcpServers = append(tcpServers, server)
		go server.ActivateAndServe()
	}

	for _, addr := range options.UDPAddresses {
		pconn, err := listenCfg.ListenPacket(ctx, addr.Network, addr.Address)
		if err != nil {
			return fmt.Errorf("error setting up dns udp listener on %s: %w", addr.Address, err)
		}
		listenAddr := addr.Address
		var packetConn net.PacketConn = pconn
		if runtime.GOOS == "illumos" {
			// needed to force net.PacketConn path instead of *net.UDPConn path
			// because of dual stack not working on illumos
			packetConn = &squashed{PacketConn: pconn}
		}
		server := &dns.Server{
			PacketConn: packetConn,
			Handler:    dnsMux,
			NotifyStartedFunc: func() {
				logger.Info("ACME DNS started", zap.String("proto", "udp"), zap.String("listen", listenAddr))
			},
		}
		udpServers = append(udpServers, server)
		go server.ActivateAndServe()
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)

	select {
	case sig := <-sigs:
		logger.Info("received signal to stop", zap.String("signal", sig.String()))
	case <-ctx.Done():
		logger.Info("context done", zap.Error(ctx.Err()))
	}

	return nil
}

type remoteKV struct {
	chord.KV
	Client protocol.KVService
}

func (r *remoteKV) PrefixList(ctx context.Context, prefix []byte) (children [][]byte, err error) {
	resp, err := r.Client.List(ctx, &protocol.PrefixRequest{
		Prefix: prefix,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetChildren(), nil
}

func chaos(version string) dns.HandlerFunc {
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
