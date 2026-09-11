package client

import (
	"context"
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
	"go.miragespace.co/specter/util/reuse"

	"github.com/quic-go/quic-go"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

type transportCfg struct {
	logger *zap.Logger
	quicTp *quic.Transport
	apex   *dialer.ParsedApex
	rtt    rtt.Recorder
}

func createTransport(cmd *cli.Command, cfg transportCfg) (*tls.Config, *overlay.QUIC) {
	clientTLSConf := &tls.Config{
		ServerName:         cfg.apex.Host,
		InsecureSkipVerify: cmd.Bool("insecure"),
		NextProtos: []string{
			tun.ALPN(protocol.Link_SPECTER_CLIENT),
		},
	}
	if v, ok := cmd.Root().Metadata["apexOverride"]; ok {
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

func cmdTunnel(ctx context.Context, cmd *cli.Command) error {
	logger := cmd.Root().Metadata["logger"].(*zap.Logger)

	cfg, err := client.NewConfig(cmd.String("config"))
	if err != nil {
		return err
	}

	parsed, err := dialer.ParseApex(cfg.Apex)
	if err != nil {
		return err
	}

	listenCfg := &net.ListenConfig{
		Control: reuse.Control,
	}

	var (
		serverListener      net.Listener
		keylessHTTPListener net.Listener
		keylessTCPListener  net.Listener
		keylessALPNMux      *overlay.ALPNMux
	)

	if cmd.IsSet("server") {
		serverListener, err = listenCfg.Listen(ctx, "tcp", cmd.String("server"))
		if err != nil {
			return err
		}
		defer serverListener.Close()
	}

	if cmd.IsSet("keyless") {
		keylessAddr := cmd.String("keyless")
		listenHost, listenPort, err := net.SplitHostPort(keylessAddr)
		if err != nil {
			return err
		}

		keylessTCPListener, err = listenCfg.Listen(ctx, "tcp", keylessAddr)
		if err != nil {
			return err
		}
		defer keylessTCPListener.Close()

		udpListener, err := listenCfg.ListenPacket(ctx, "udp", keylessAddr)
		if err != nil {
			return err
		}
		defer udpListener.Close()

		if listenPort == "443" {
			keylessHTTPListener, err = listenCfg.Listen(ctx, "tcp", fmt.Sprintf("%s:%d", listenHost, 80))
			if err != nil {
				return fmt.Errorf("error setting up http listener: %w", err)
			}
			defer keylessHTTPListener.Close()
		}

		qTr := &quic.Transport{Conn: udpListener}
		defer qTr.Close()

		keylessALPNMux, err = overlay.NewMux(qTr)
		if err != nil {
			return err
		}
		defer keylessALPNMux.Close()

		go keylessALPNMux.Accept(ctx)
	}

	listener, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return err
	}
	defer listener.Close()

	quicTransport := &quic.Transport{Conn: listener}
	defer quicTransport.Close()

	transportRTT := rttImpl.NewInstrumentation(20)
	tlsCfg, transport := createTransport(cmd, transportCfg{
		logger: logger,
		quicTp: quicTransport,
		apex:   parsed,
		rtt:    transportRTT,
	})
	defer transport.Stop()

	pkiClient := dialer.GetPKIClient(tlsCfg.Clone(), parsed)

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)

	c, err := client.NewClient(ctx, client.ClientConfig{
		Logger:          logger,
		Configuration:   cfg,
		PKIClient:       pkiClient,
		ServerTransport: transport,
		Recorder:        transportRTT,
		ReloadSignal:    s,
		ServerListener:  serverListener,
		KeylessProxy: client.KeylessProxyConfig{
			HTTPListner:  keylessHTTPListener,
			HTTPSListner: keylessTCPListener,
			ALPNMux:      keylessALPNMux,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to bootstrap client: %w", err)
	}
	defer c.Close()

	if err := c.Register(ctx); err != nil {
		return fmt.Errorf("failed to register client: %w", err)
	}

	if err := c.Initialize(ctx, true); err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}

	c.Start(ctx)

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
