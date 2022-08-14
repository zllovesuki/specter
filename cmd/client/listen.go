package client

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

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

	test, err := getConnection(dial)
	if err != nil {
		return err
	}
	test.Close()

	listener, err := net.Listen("tcp", ctx.String("listen"))
	if err != nil {
		return err
	}
	defer listener.Close()

	logger.Info("listening for new connections", zap.String("addr", listener.Addr().String()))

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
		logger.Info("New incoming connection", zap.String("addr", conn.RemoteAddr().String()))
		go func(local *net.TCPConn) {
			remote, err := getConnection(dial)
			if err != nil {
				logger.Error("Error dialing specter gateway for new connection", zap.Error(err))
				return
			}

			go func() {
				_, err := local.ReadFrom(remote)
				if err != nil {
					logger.Error("error piping to target", zap.Error(err))
				}
			}()
			go func() {
				_, err := io.Copy(remote, local)
				if err != nil {
					logger.Error("error piping to target", zap.Error(err))
				}
			}()
		}(conn.(*net.TCPConn))
	}
}
