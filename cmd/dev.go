package cmd

import (
	"context"

	"github.com/mholt/acmez/acme"
	"go.uber.org/zap"
)

type NoopSolver struct {
	logger *zap.Logger
}

func (s NoopSolver) Present(ctx context.Context, chal acme.Challenge) error {
	s.logger.Debug("solver present challenge", zap.Any("challenge", chal))
	return nil
}

func (s NoopSolver) CleanUp(ctx context.Context, chal acme.Challenge) error {
	s.logger.Debug("solver cleanup challenge", zap.Any("challenge", chal))
	return nil
}
