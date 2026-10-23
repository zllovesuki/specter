package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"time"

	"go.miragespace.co/specter/acme"
	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/internal/listen"
	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/util/reuse"

	"github.com/avast/retry-go/v5"
	"github.com/miekg/dns"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

type Options struct {
	Logger       *zap.Logger
	Version      string
	TCPAddresses []listen.Address
	UDPAddresses []listen.Address
	RPCNetwork   string
	RPCAddress   string
	ACMEEmail    string
	ACMEZone     string
}

// Run serves the ACME challenge zone until ctx is canceled.
func Run(ctx context.Context, options Options) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
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
	configClient := config.NewClient("http://rpc", httpClient)
	loadConfig := func(ctx context.Context) (config.Record, error) {
		return loadDNSConfig(ctx, configClient)
	}
	record, err := waitDNSConfig(ctx, logger, loadConfig, acmeDomain, dnsConfigPollInterval)
	if err != nil {
		return err
	}
	acmeDNS, err := acme.NewDNS(ctx, logger.With(zap.String("component", "acme_dns")),
		kv,
		options.ACMEEmail,
		acmeDomain, record,
	)
	if err != nil {
		return fmt.Errorf("configure ACME DNS: %w", err)
	}
	refreshDone := make(chan struct{})
	go func() {
		defer close(refreshDone)
		refreshDNSConfig(ctx, logger, loadConfig, acmeDNS, dnsConfigPollInterval)
	}()
	defer func() {
		cancel()
		<-refreshDone
	}()

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

	<-ctx.Done()
	logger.Info("ACME DNS stopping", zap.Error(ctx.Err()))

	return nil
}

const (
	dnsConfigPollInterval = 5 * time.Second
	dnsConfigTimeout      = 3 * time.Second
)

func loadDNSConfig(ctx context.Context, client *config.Client) (config.Record, error) {
	ctx, cancel := context.WithTimeout(ctx, dnsConfigTimeout)
	defer cancel()
	record, err := client.Get(ctx, acme.DNSConfigName)
	var rpcError twirp.Error
	if errors.As(err, &rpcError) && rpcError.Code() == twirp.NotFound {
		return config.Record{}, fmt.Errorf("shared DNS configuration is missing; initialize the first server with specter config init --data-dir DIR --file FILE")
	}
	return record, err
}

func waitDNSConfig(ctx context.Context, logger *zap.Logger, load func(context.Context) (config.Record, error), localZone string, interval time.Duration) (config.Record, error) {
	return retry.NewWithData[config.Record](
		retry.Context(ctx),
		retry.UntilSucceeded(),
		retry.Delay(interval),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(_ uint, err error) {
			if ctx.Err() == nil {
				logger.Warn("Waiting for shared DNS configuration", zap.Error(err))
			}
		}),
	).Do(func() (config.Record, error) {
		record, err := load(ctx)
		if err == nil {
			_, err = acme.DecodeDNSConfig(record, localZone)
		}
		return record, err
	})
}

func refreshDNSConfig(ctx context.Context, logger *zap.Logger, load func(context.Context) (config.Record, error), handler *acme.DNS, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			record, err := load(ctx)
			if err == nil {
				err = handler.ApplyConfig(record)
			}
			if err != nil && ctx.Err() == nil {
				logger.Warn("Unable to refresh shared DNS configuration; retaining the active version", zap.String("version", handler.ConfigVersion()), zap.Error(err))
			}
		}
	}
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
