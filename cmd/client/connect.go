package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"

	"github.com/libp2p/go-yamux/v3"
	"github.com/lucas-clemente/quic-go"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

type transportDialer func() (io.ReadWriteCloser, error)

func cmdConnect(ctx *cli.Context) error {
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

	rw, err := getConnection(dial)
	if err != nil {
		return err
	}
	defer rw.Close()

	logger.Info("Tunnel established")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		_, err := io.Copy(os.Stdout, rw)
		if err != nil {
			logger.Error("error piping to target", zap.Error(err))
			sigs <- syscall.SIGTERM
		}
	}()
	go func() {
		_, err := io.Copy(rw, os.Stdin)
		if err != nil {
			logger.Error("error piping to target", zap.Error(err))
			sigs <- syscall.SIGTERM
		}
	}()

	logger.Info("received signal to stop", zap.String("signal", (<-sigs).String()))

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

func getConnection(dial transportDialer) (io.ReadWriteCloser, error) {
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

func tlsDialer(ctx *cli.Context, logger *zap.Logger, parsed *parsedApex) (transportDialer, error) {
	clientTLSConf := &tls.Config{
		ServerName:         parsed.host,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NextProtos: []string{
			tun.ALPN(protocol.Link_TCP),
		},
	}
	// used in integration test
	if v, ok := ctx.App.Metadata["connectOverride"]; ok {
		clientTLSConf.ServerName = v.(string)
	}

	dialer := &tls.Dialer{
		Config: clientTLSConf,
	}

	openCtx, cancel := context.WithTimeout(ctx.Context, transport.ConnectTimeout)
	defer cancel()

	conn, err := dialer.DialContext(openCtx, "tcp", parsed.String())
	if err != nil {
		return nil, err
	}

	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	session, err := yamux.Client(conn, cfg, nil)
	if err != nil {
		return nil, err
	}

	return func() (io.ReadWriteCloser, error) {
		return session.OpenStream(ctx.Context)
	}, nil
}

func quicDialer(ctx *cli.Context, logger *zap.Logger, parsed *parsedApex) (transportDialer, error) {
	clientTLSConf := &tls.Config{
		ServerName:         parsed.host,
		InsecureSkipVerify: ctx.Bool("insecure"),
		NextProtos: []string{
			tun.ALPN(protocol.Link_TCP),
		},
	}
	// used in integration test
	if v, ok := ctx.App.Metadata["connectOverride"]; ok {
		clientTLSConf.ServerName = v.(string)
	}
	q, err := quic.DialAddrContext(ctx.Context, parsed.String(), clientTLSConf, &quic.Config{
		KeepAlivePeriod:      time.Second * 5,
		HandshakeIdleTimeout: transport.ConnectTimeout,
		MaxIdleTimeout:       time.Second * 30,
		EnableDatagrams:      true,
	})
	if err != nil {
		return nil, err
	}

	return func() (io.ReadWriteCloser, error) {
		return q.OpenStream()
	}, nil
}
