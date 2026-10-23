package client

import (
	clientruntime "go.miragespace.co/specter/internal/client"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func runtimeOptions(cmd *cli.Command) clientruntime.Options {
	opts := clientruntime.Options{
		Logger:             cmd.Root().Metadata["logger"].(*zap.Logger),
		InsecureSkipVerify: cmd.Bool("insecure"),
	}
	// Used by development images and integration tests.
	if override, ok := cmd.Root().Metadata["apexOverride"]; ok {
		serverName := override.(string)
		opts.ServerNameOverride = &serverName
	}
	return opts
}
