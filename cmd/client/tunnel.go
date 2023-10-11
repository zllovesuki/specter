package client

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.miragespace.co/specter/overlay"
	rttImpl "go.miragespace.co/specter/rtt"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/tun/client"
	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/quic-go/quic-go"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

type transportCfg struct {
	logger *zap.Logger
	quicTp *quic.Transport
	apex   *dialer.ParsedApex
	rtt    rtt.Recorder
}

func createTransport(ctx *cli.Context, cfg transportCfg) (*tls.Config, *overlay.QUIC) {
	clientTLSConf := &tls.Config{
		ServerName:         cfg.apex.Host,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NextProtos: []string{
			tun.ALPN(protocol.Link_SPECTER_CLIENT),
		},
	}
	if v, ok := ctx.App.Metadata["apexOverride"]; ok {
		clientTLSConf.ServerName = v.(string)
	}
	return clientTLSConf, overlay.NewQUIC(overlay.TransportConfig{
		Logger:        cfg.logger,
		QuicTransport: cfg.quicTp,
		Endpoint:      &protocol.Node{},
		ClientTLS:     clientTLSConf,
		RTTRecorder:   cfg.rtt,
	})
}

func cmdTunnel(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	cfg, err := client.NewConfig(ctx.String("config"))
	if err != nil {
		return err
	}

	parsed, err := dialer.ParseApex(cfg.Apex)
	if err != nil {
		return err
	}

	var serverListener net.Listener
	if ctx.IsSet("server") {
		listenCfg := &net.ListenConfig{}
		serverListener, err = listenCfg.Listen(ctx.Context, "tcp", ctx.String("server"))
		if err != nil {
			return err
		}
		defer serverListener.Close()
	}

	listener, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return err
	}
	defer listener.Close()

	quicTransport := &quic.Transport{Conn: listener}
	defer quicTransport.Close()

	transportRTT := rttImpl.NewInstrumentation(20)
	tlsCfg, transport := createTransport(ctx, transportCfg{
		logger: logger,
		quicTp: quicTransport,
		apex:   parsed,
		rtt:    transportRTT,
	})
	defer transport.Stop()

	pkiClient := dialer.GetPKIClient(tlsCfg.Clone(), parsed)

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)

	c, err := client.NewClient(ctx.Context, client.ClientConfig{
		Logger:          logger,
		Configuration:   cfg,
		PKIClient:       pkiClient,
		ServerTransport: transport,
		Recorder:        transportRTT,
		ReloadSignal:    s,
		ServerListener:  serverListener,
	})
	if err != nil {
		return fmt.Errorf("failed to bootstrap client: %w", err)
	}
	defer c.Close()

	if err := c.Register(ctx.Context); err != nil {
		return fmt.Errorf("failed to register client: %w", err)
	}

	if err := c.Initialize(ctx.Context, true); err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}

	c.Start(ctx.Context)

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
