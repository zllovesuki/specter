package client

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

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
		dial   TransportDialer
		err    error
	)

	parsed, err := ParseApex(hostname)
	if err != nil {
		return fmt.Errorf("error parsing hostname: %w", err)
	}

	if ctx.IsSet("tcp") {
		remote, dial, err = TLSDialer(ctx, logger, parsed, true)
	} else {
		remote, dial, err = QuicDialer(ctx, logger, parsed, true)
	}
	if err != nil {
		return fmt.Errorf("error dialing specter gateway: %w", err)
	}

	rw, err := getConnection(dial)
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

func statusExchange(rw io.ReadWriter) (*protocol.TunnelStatus, error) {
	// because of quic's early connection, the client need to "poke" the gateway before
	// the gateway can actually accept a stream, despite .OpenStreamSync
	status := &protocol.TunnelStatus{}
	err := rpc.Send(rw, status)
	if err != nil {
		return nil, fmt.Errorf("error sending status check: %w", err)
	}
	status.Reset()
	err = rpc.Receive(rw, status)
	if err != nil {
		return nil, fmt.Errorf("error receiving checking status: %w", err)
	}
	return status, nil
}

func getConnection(dial TransportDialer) (net.Conn, error) {
	rw, err := dial()
	if err != nil {
		return nil, err
	}
	status, err := statusExchange(rw)
	if err != nil {
		return nil, err
	}
	if status.GetStatus() != protocol.TunnelStatusCode_STATUS_OK {
		return nil, fmt.Errorf("error opening tunnel: %s", status.Error)
	}
	return rw, nil
}
