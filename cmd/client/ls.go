package client

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	clientruntime "go.miragespace.co/specter/internal/client"

	"github.com/urfave/cli/v3"
)

func cmdLs(ctx context.Context, cmd *cli.Command) error {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)
	defer signal.Stop(s)

	c, closeClient, err := clientruntime.OpenOwner(ctx, cmd.String("config"), runtimeOptions(cmd))
	if err != nil {
		return err
	}
	defer closeClient()

	hostnames, err := c.GetRegisteredHostnames(ctx)
	if err != nil {
		return err
	}

	c.FormatList(hostnames, os.Stdout)

	return nil
}
