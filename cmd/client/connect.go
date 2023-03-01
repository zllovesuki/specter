package client

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/tun/client/connector"
	"kon.nect.sh/specter/tun/client/dialer"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

const (
	PipeInKey  string = "pipeIn"
	PipeOutKey string = "pipeOut"
)

func cmdConnect(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	out, ok := ctx.App.Metadata[PipeOutKey].(io.Writer)
	if !ok || out == nil {
		return fmt.Errorf("missing pipe output")
	}
	in, ok := ctx.App.Metadata[PipeInKey].(io.Reader)
	if !ok || in == nil {
		return fmt.Errorf("missing pipe input")
	}

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
		remote, dial, err = tlsDialer(ctx, logger, parsed, true)
	} else {
		remote, dial, err = quicDialer(ctx, logger, parsed, true)
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
		_, err := io.Copy(out, rw)
		if err != nil {
			logger.Error("error piping to target", zap.Error(err))
			sigs <- syscall.SIGTERM
		}
	}()
	go func() {
		_, err := io.Copy(rw, in)
		if err != nil {
			logger.Error("error piping to target", zap.Error(err))
			sigs <- syscall.SIGTERM
		}
	}()

	select {
	case sig := <-sigs:
		logger.Info("received signal to stop", zap.String("signal", sig.String()))
	case <-ctx.Context.Done():
		logger.Info("context done", zap.Error(ctx.Context.Err()))
	}

	return nil
}
