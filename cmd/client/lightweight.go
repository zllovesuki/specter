package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"go.miragespace.co/specter/tun/client"
	"go.miragespace.co/specter/tun/client/dialer"

	"github.com/quic-go/quic-go"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func lightweightFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "apex",
			Required: true,
			Usage:    "TLS authority, bootstrap endpoint, and generated URL port; the server reports the URL hostname",
		},
	}
}

func loadTunnelToken(path, env string, envSet bool) (string, error) {
	if (path != "") == envSet {
		return "", fmt.Errorf("specify exactly one of --token-file or SPECTER_TUNNEL_TOKEN")
	}
	data := []byte(env)
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		data, err = io.ReadAll(io.LimitReader(f, 4097))
		if err != nil {
			return "", err
		}
	}
	if len(data) > 4096 {
		return "", fmt.Errorf("tunnel token input exceeds 4 KiB")
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("tunnel token is empty")
	}
	return token, nil
}

func runLightweight(ctx context.Context, cmd *cli.Command, token string) error {
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("expected exactly one target URL")
	}
	apex, err := dialer.ParseApex(cmd.String("apex"))
	if err != nil {
		return err
	}
	logger := cmd.Root().Metadata["logger"].(*zap.Logger)
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	listener, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return err
	}
	defer listener.Close()
	quicTransport := &quic.Transport{Conn: listener}
	defer quicTransport.Close()
	tlsCfg, tp := createTransport(cmd, transportCfg{
		logger: logger,
		quicTp: quicTransport,
		apex:   apex,
	})
	defer tp.Stop()
	l, err := client.NewLightweightClient(client.LightweightConfig{
		Logger:    logger,
		Transport: tp,
		PKIClient: dialer.GetPKIClient(tlsCfg.Clone(), apex),
		Apex:      apex,
		Target:    cmd.Args().First(),
		Token:     token,
		Output:    cmd.Root().Writer,
	})
	if err != nil {
		return err
	}
	return l.Run(ctx)
}
