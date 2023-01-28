package specter

import (
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"kon.nect.sh/specter/cmd/client"
	"kon.nect.sh/specter/cmd/server"
	"kon.nect.sh/specter/spec/errata"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Build = "head"
)

var (
	App = cli.App{
		Name:            "specter",
		Usage:           fmt.Sprintf("build for %s on %s", runtime.GOARCH, runtime.GOOS),
		Version:         Build,
		HideHelpCommand: true,
		Copyright:       "miragespace.com, licensed under MIT.\nSee https://github.com/zllovesuki/specter/blob/main/ThirdPartyLicenses.txt for third-party licenses.",
		Description:     "like ngrok, but ambitious and also with DHT for flavor",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "verbose",
				Value: false,
				Usage: "enable verbose logging",
			},
			&cli.Int64Flag{
				Name:   "rand",
				Hidden: true,
				Value:  time.Now().Unix(),
			},
		},
		Commands: []*cli.Command{
			server.Cmd,
			client.Cmd,
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

	seed := ctx.Int64("rand")
	logger.Info("specter: seeding math/rand", zap.Int64("rand", seed), zap.Bool("overriden", ctx.IsSet("rand")))
	rand.Seed(seed)

	return ConfigApp(ctx)
}

func ConfigApp(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)
	if errata.ConfigDNS() {
		logger.Info("errata: net.Resolver configured with custom dialer")
	}
	return nil
}
