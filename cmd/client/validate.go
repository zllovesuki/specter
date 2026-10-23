package client

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	clientruntime "go.miragespace.co/specter/internal/client"
	"go.miragespace.co/specter/spec/acme"

	"github.com/urfave/cli/v3"
)

func cmdValidate(ctx context.Context, cmd *cli.Command) error {
	hostname := cmd.Args().First()
	if hostname == "" {
		return fmt.Errorf("missing hostname in argument")
	}
	hostname, err := acme.Normalize(hostname)
	if err != nil {
		return err
	}

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)
	defer signal.Stop(s)

	c, closeClient, err := clientruntime.OpenOwner(ctx, cmd.String("config"), runtimeOptions(cmd))
	if err != nil {
		return err
	}
	defer closeClient()

	resp, err := c.RequestAcmeValidation(ctx, hostname)
	if err != nil {
		return fmt.Errorf("failed to validate acme setup: %w", err)
	}

	fmt.Printf("\n")
	c.FormatValidate(hostname, resp, os.Stdout)

	return nil
}
