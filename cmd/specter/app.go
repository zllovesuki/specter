package specter

import (
	"fmt"
	"runtime"

	"go.miragespace.co/specter/cmd/client"
	"go.miragespace.co/specter/cmd/dns"
	"go.miragespace.co/specter/cmd/server"
	"go.miragespace.co/specter/spec"
	"go.miragespace.co/specter/spec/errata"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	App = cli.App{
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

			&cli.BoolFlag{
				Name:    "doh",
				Hidden:  true,
				Value:   true,
				EnvVars: []string{"DOH"},
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

func ConfigLogger(ctx *cli.Context) error {
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
	_, err = zap.RedirectStdLogAt(logger.With(zap.String("subsystem", "unknown")), zapcore.InfoLevel)
	if err != nil {
		return fmt.Errorf("redirecting stdlog output: %w", err)
	}
	ctx.App.Metadata["logger"] = logger

	return ConfigApp(ctx)
}

func ConfigApp(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)
	if errata.ConfigDNS(ctx.Bool("doh")) {
		logger.Debug("errata: net.DefaultResolver configured with DoH dialer")
	}
	if errata.ConfigUDPRecvBuffer() {
		logger.Debug("errata: net.core.rmem_max is set to 33554432 (32MiB)")
	}
	if errata.ConfigUDPSendBuffer() {
		logger.Debug("errata: net.core.wmem_max is set to 33554432 (32MiB)")
	}
	return nil
}
