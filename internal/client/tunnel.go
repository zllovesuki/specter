package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.miragespace.co/specter/overlay"
	rttImpl "go.miragespace.co/specter/rtt"
	"go.miragespace.co/specter/tun/client"
	"go.miragespace.co/specter/tun/client/dialer"
	"go.miragespace.co/specter/util/reuse"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

type TunnelOptions struct {
	Options
	ConfigPath     string
	ServerAddress  *string
	KeylessAddress *string
}

func RunTunnel(ctx context.Context, opts TunnelOptions) error {
	cfg, apex, err := loadOwnerConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	listenCfg := &net.ListenConfig{Control: reuse.Control}
	var (
		serverListener      net.Listener
		keylessHTTPListener net.Listener
		keylessTCPListener  net.Listener
		keylessALPNMux      *overlay.ALPNMux
	)
	if opts.ServerAddress != nil {
		serverListener, err = listenCfg.Listen(ctx, "tcp", *opts.ServerAddress)
		if err != nil {
			return err
		}
		defer serverListener.Close()
	}
	if opts.KeylessAddress != nil {
		keylessAddr := *opts.KeylessAddress
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
		quicTransport := &quic.Transport{Conn: udpListener}
		defer quicTransport.Close()
		keylessALPNMux, err = overlay.NewMux(quicTransport)
		if err != nil {
			return err
		}
		defer keylessALPNMux.Close()
		go keylessALPNMux.Accept(ctx)
	}

	transportRTT := rttImpl.NewInstrumentation(20)
	tlsConfig, transport, closeTransport, err := openTransport(opts.Options, apex, transportRTT)
	if err != nil {
		return err
	}
	defer closeTransport()

	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)
	defer signal.Stop(reload)
	c, err := client.NewClient(ctx, client.ClientConfig{
		Logger:          opts.Logger,
		Configuration:   cfg,
		PKIClient:       dialer.GetPKIClient(tlsConfig.Clone(), apex),
		ServerTransport: transport,
		Recorder:        transportRTT,
		ReloadSignal:    reload,
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
	defer signal.Stop(sigs)
	select {
	case sig := <-sigs:
		opts.Logger.Info("received signal to stop", zap.String("signal", sig.String()))
	case <-ctx.Done():
		opts.Logger.Info("context done", zap.Error(ctx.Err()))
	}
	return nil
}
