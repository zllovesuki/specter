package specter

import (
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"go.miragespace.co/specter/cmd/client"
	"go.miragespace.co/specter/cmd/dns"
	"go.miragespace.co/specter/cmd/server"
	"go.miragespace.co/specter/spec/errata"

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
		Description:     "like ngrok, but more ambitious with DHT for flavor",
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

	seed := ctx.Int64("rand")
	logger.Debug("specter: seeding math/rand", zap.Int64("rand", seed), zap.Bool("overridden", ctx.IsSet("rand")))
	rand.Seed(seed)

	return ConfigApp(ctx)
}

func ConfigApp(ctx *cli.Context) error {
	logger := ctx.App.Metadata["logger"].(*zap.Logger)
	if errata.ConfigDNS(ctx.Bool("doh")) {
		logger.Debug("errata: net.DefaultResolver configured with DoH dialer")
	}
	if errata.ConfigUDPBuffer() {
		logger.Debug("errata: net.core.rmem_max is set to 2500000")
	}
	return nil
}
