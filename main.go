package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"kon.nect.sh/specter/cmd/client"
	"kon.nect.sh/specter/cmd/server"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Build = "head"
)

func main() {
	app := cli.App{
		Name:            "specter",
		Usage:           fmt.Sprintf("build for %s on %s", runtime.GOARCH, runtime.GOOS),
		Version:         Build,
		HideHelpCommand: true,
		Copyright:       "miragespace.com, licensed under MIT.",
		Description:     "specter, a distributed networking and KV toolkit",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "verbose",
				Value: false,
				Usage: "enable verbose logging",
			},
		},
		Commands: []*cli.Command{
			server.Cmd,
			client.Cmd,
		},
		Before: func(ctx *cli.Context) error {
			var config zap.Config
			if ctx.Bool("verbose") {
				config = zap.NewDevelopmentConfig()
				config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
			} else {
				config = zap.NewProductionConfig()
			}
			// Redirect everything to stderr
			config.OutputPaths = []string{"stderr"}
			logger, err := config.Build()
			if err != nil {
				return err
			}
			_, err = zap.RedirectStdLogAt(logger, zapcore.InfoLevel)
			if err != nil {
				return fmt.Errorf("redirecting stdlog output: %w", err)
			}
			ctx.App.Metadata["logger"] = logger
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
