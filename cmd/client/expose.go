package client

import (
	"context"

	"github.com/urfave/cli/v3"
)

func exposeCommand() *cli.Command {
	return &cli.Command{
		Name:      "expose",
		Usage:     "Serve one target at a temporary URL",
		ArgsUsage: "<target>",
		Flags:     lightweightFlags(),
		Action:    func(ctx context.Context, cmd *cli.Command) error { return runLightweight(ctx, cmd, "") },
	}
}
