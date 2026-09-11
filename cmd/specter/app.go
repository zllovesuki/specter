package specter

import (
	"context"
	"fmt"
	"runtime"

	"go.miragespace.co/specter/cmd/client"
	"go.miragespace.co/specter/cmd/dns"
	"go.miragespace.co/specter/cmd/server"
	"go.miragespace.co/specter/spec"
	"go.miragespace.co/specter/spec/errata"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	App = cli.Command{
		Name:        "specter",
		Usage:       fmt.Sprintf("build for %s on %s", runtime.GOARCH, runtime.GOOS),
		Version:     spec.BuildVersion,
		Copyright:   "miragespace.com, licensed under MIT.\nSee https://github.com/zllovesuki/specter/blob/main/ThirdPartyLicenses.txt for third-party licenses.",
		Description: "like ngrok, but more ambitious with DHT for flavor",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "verbose",
				Value: false,
				Usage: "enable verbose logging",
			},
		},
		Commands: []*cli.Command{
			dns.Generate(),
			server.Generate(),
			client.Generate(),
		},
		Before: ConfigLogger,
	}
)

func ConfigLogger(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	var config zap.Config
	if cmd.Bool("verbose") {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
	}
	// Redirect everything to stderr
	config.OutputPaths = []string{"stderr"}
	logger, err := config.Build()
	if err != nil {
		return ctx, err
	}
	_, err = zap.RedirectStdLogAt(logger.With(zap.String("subsystem", "unknown")), zapcore.InfoLevel)
	if err != nil {
		return ctx, fmt.Errorf("redirecting stdlog output: %w", err)
	}
	cmd.Root().Metadata["logger"] = logger

	return ctx, ConfigApp(cmd)
}

func ConfigApp(cmd *cli.Command) error {
	logger := cmd.Root().Metadata["logger"].(*zap.Logger)
	if errata.ConfigUDPRecvBuffer() {
		logger.Debug("errata: net.core.rmem_max is set to 33554432 (32MiB)")
	}
	if errata.ConfigUDPSendBuffer() {
		logger.Debug("errata: net.core.wmem_max is set to 33554432 (32MiB)")
	}
	return nil
}
