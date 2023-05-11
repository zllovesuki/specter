package client

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/overlay"
	rttImpl "kon.nect.sh/specter/rtt"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rtt"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/client"
	"kon.nect.sh/specter/tun/client/dialer"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func createTransport(ctx *cli.Context, logger *zap.Logger, cfg *client.Config, apex *dialer.ParsedApex, r rtt.Recorder) (*tls.Config, *overlay.QUIC) {
	clientTLSConf := &tls.Config{
		ServerName:         apex.Host,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NextProtos: []string{
			tun.ALPN(protocol.Link_SPECTER_CLIENT),
		},
	}
	if v, ok := ctx.App.Metadata["apexOverride"]; ok {
		clientTLSConf.ServerName = v.(string)
	}
	return clientTLSConf, overlay.NewQUIC(overlay.TransportConfig{
		Logger:      logger,
		Endpoint:    &protocol.Node{},
		ClientTLS:   clientTLSConf,
		RTTRecorder: r,
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

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)

	transportRTT := rttImpl.NewInstrumentation(20)
	tlsCfg, transport := createTransport(ctx, logger, cfg, parsed, transportRTT)
	defer transport.Stop()

	pkiClient := dialer.GetPKIClient(tlsCfg.Clone(), parsed)

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
