package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.miragespace.co/specter/tun/client/connector"
	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func cmdListen(ctx context.Context, cmd *cli.Command) error {
	logger := cmd.Root().Metadata["logger"].(*zap.Logger)

	hostname := cmd.Args().First()
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

	if cmd.IsSet("tcp") {
		remote, dial, err = tlsDialer(ctx, cmd, logger, parsed, false)
	} else {
		remote, dial, err = quicDialer(ctx, cmd, logger, parsed, false)
	}
	if err != nil {
		return fmt.Errorf("error dialing specter gateway: %w", err)
	}

	listener, err := net.Listen("tcp", cmd.String("listen"))
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
	case <-ctx.Done():
		logger.Info("context done", zap.Error(ctx.Err()))
	}

	return nil
}
