package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.miragespace.co/specter/tun/client/connector"
	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func cmdConnect(ctx context.Context, cmd *cli.Command) error {
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
		remote, dial, err = tlsDialer(ctx, cmd, logger, parsed, true)
	} else {
		remote, dial, err = quicDialer(ctx, cmd, logger, parsed, true)
	}
	if err != nil {
		return fmt.Errorf("error dialing specter gateway: %w", err)
	}

	rw, err := connector.GetConnection(dial)
	if err != nil {
		return err
	}
	defer rw.Close()

	logger.Info("Tunnel established", zap.String("via", remote.String()))

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		_, err := io.Copy(cmd.Root().Writer, rw)
		if err != nil {
			logger.Error("error piping to target", zap.Error(err))
			sigs <- syscall.SIGTERM
		}
	}()
	go func() {
		_, err := io.Copy(rw, cmd.Root().Reader)
		if err != nil {
			logger.Error("error piping to target", zap.Error(err))
			sigs <- syscall.SIGTERM
		}
	}()

	select {
	case sig := <-sigs:
		logger.Info("received signal to stop", zap.String("signal", sig.String()))
	case <-ctx.Done():
		logger.Info("context done", zap.Error(ctx.Err()))
	}

	return nil
}
