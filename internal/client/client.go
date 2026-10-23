package client

import (
	"crypto/tls"
	"net"

	"go.miragespace.co/specter/overlay"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/tun/client"
	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

type Options struct {
	Logger             *zap.Logger
	InsecureSkipVerify bool
	ServerNameOverride *string
}

func loadOwnerConfig(path string) (*client.Config, *dialer.ParsedApex, error) {
	cfg, err := client.NewConfig(path)
	if err != nil {
		return nil, nil, err
	}
	apex, err := dialer.ParseApex(cfg.Apex)
	if err != nil {
		return nil, nil, err
	}
	return cfg, apex, nil
}

func openTransport(opts Options, apex *dialer.ParsedApex, recorder rtt.Recorder) (*tls.Config, *overlay.QUIC, func(), error) {
	listener, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, nil, nil, err
	}
	quicTransport := &quic.Transport{Conn: listener}
	tlsConfig := &tls.Config{
		ServerName:         apex.Host,
		InsecureSkipVerify: opts.InsecureSkipVerify,
		NextProtos: []string{
			tun.ALPN(protocol.Link_SPECTER_CLIENT),
		},
	}
	if opts.ServerNameOverride != nil {
		tlsConfig.ServerName = *opts.ServerNameOverride
	}
	transport := overlay.NewQUIC(overlay.TransportConfig{
		Logger:        opts.Logger,
		QuicTransport: quicTransport,
		Endpoint:      &protocol.Node{},
		ClientTLS:     tlsConfig,
		RTTRecorder:   recorder,
	})
	closeTransport := func() {
		transport.Stop()
		quicTransport.Close()
		listener.Close()
	}
	return tlsConfig, transport, closeTransport, nil
}
