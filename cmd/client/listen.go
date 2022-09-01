package client

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/spec/tun"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func cmdListen(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)

	hostname := ctx.Args().First()
	if hostname == "" {
		return fmt.Errorf("missing hostname in argument")
	}

	var dial transportDialer
	var err error

	parsed, err := parseApex(hostname)
	if err != nil {
		return fmt.Errorf("error parsing hostname: %w", err)
	}

	if ctx.IsSet("tcp") {
		dial, err = tlsDialer(ctx, logger, parsed)
	} else {
		dial, err = quicDialer(ctx, logger, parsed)
	}
	if err != nil {
		return fmt.Errorf("error dialing specter gateway: %w", err)
	}

	test, _, err := getConnection(dial)
	if err != nil {
		return err
	}
	test.Close()

	listener, err := net.Listen("tcp", ctx.String("listen"))
	if err != nil {
		return err
	}
	defer listener.Close()

	logger.Info("listening for local connections", zap.String("listen", listener.Addr().String()))

	go handleConnections(logger, listener, dial)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("received signal to stop", zap.String("signal", (<-sigs).String()))

	return nil
}

func handleConnections(logger *zap.Logger, listener net.Listener, dial transportDialer) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(local *net.TCPConn) {
			r, remote, err := getConnection(dial)
			if err != nil {
				logger.Error("Error forwarding local connections via specter gateway", zap.Error(err))
				local.Close()
				return
			}

			logger.Info("Forwarding incoming connection", zap.String("local", conn.RemoteAddr().String()), zap.String("via", remote.String()))

			go func() {
				errChan := tun.Pipe(r, local)
				for err := range errChan {
					logger.Error("error piping to target", zap.Error(err))
				}
			}()
		}(conn.(*net.TCPConn))
	}
}
