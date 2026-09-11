package client

import (
	"context"
	"fmt"

	"go.miragespace.co/specter/tun/client"

	"github.com/urfave/cli/v3"
)

func cmdConfigExample(ctx context.Context, cmd *cli.Command) error {
	fmt.Fprintf(cmd.Root().Writer, "%s", client.ExampleConfigYAML())
	return nil
}
