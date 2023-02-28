package client

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/tun/client/dialer"

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

	go HandleConnections(logger, listener, dial)

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

func HandleConnections(logger *zap.Logger, listener net.Listener, dial dialer.TransportDialer) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(local *net.TCPConn) {
			r, err := getConnection(dial)
			if err != nil {
				logger.Error("Error forwarding local connections via specter gateway", zap.Error(err))
				local.Close()
				return
			}

			logger.Info("Forwarding incoming connection", zap.String("local", conn.RemoteAddr().String()), zap.String("via", r.RemoteAddr().String()))

			go func() {
				errChan := tun.Pipe(r, local)
				for err := range errChan {
					logger.Error("error piping to target", zap.Error(err))
				}
			}()
		}(conn.(*net.TCPConn))
	}
}
