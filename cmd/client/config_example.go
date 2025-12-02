package client

import (
	"fmt"

	"go.miragespace.co/specter/tun/client"

	"github.com/urfave/cli/v2"
)

func cmdConfigExample(ctx *cli.Context) error {
	fmt.Fprintf(ctx.App.Writer, "%s", client.ExampleConfigYAML())
	return nil
}
