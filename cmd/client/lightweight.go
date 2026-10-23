package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	clientruntime "go.miragespace.co/specter/internal/client"

	"github.com/urfave/cli/v3"
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
	return clientruntime.RunLightweight(ctx, clientruntime.LightweightOptions{
		Options: runtimeOptions(cmd),
		Apex:    cmd.String("apex"),
		Target:  cmd.Args().First(),
		Token:   token,
		Output:  cmd.Root().Writer,
	})
}
