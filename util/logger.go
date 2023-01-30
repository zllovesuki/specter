package util

import (
	"fmt"
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func GetStdLogger(parent *zap.Logger, sub string) *log.Logger {
	logger, err := zap.NewStdLogAt(parent.With(zap.String("subsystem", sub)), zapcore.WarnLevel)
	if err != nil {
		panic(fmt.Errorf("error getting logger: %w", err))
	}
	return logger
}
