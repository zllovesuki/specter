package client

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.miragespace.co/specter/tun/client/connector"
	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func cmdListen(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	hostname := ctx.Args().First()
	if hostname == "" {
		return fmt.Errorf("missing hostname in argument")
	}

	var (
		remote net.Addr
		dial   dialer.TransportDialer
		err    error
	)

	parsed, err := dialer.ParseApex(hostname)
	if err != nil {
		return fmt.Errorf("error parsing hostname: %w", err)
	}

	if ctx.IsSet("tcp") {
		remote, dial, err = tlsDialer(ctx, logger, parsed, false)
	} else {
		remote, dial, err = quicDialer(ctx, logger, parsed, false)
	}
	if err != nil {
		return fmt.Errorf("error dialing specter gateway: %w", err)
	}

	listener, err := net.Listen("tcp", ctx.String("listen"))
	if err != nil {
		return err
	}
	defer listener.Close()

	logger.Info("listening for local connections", zap.String("listen", listener.Addr().String()), zap.String("via", remote.String()))

	go connector.HandleConnections(logger, listener, dial)

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
