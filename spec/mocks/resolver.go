package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type Resolver struct {
	mock.Mock
}

func (r *Resolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	args := r.Called(ctx, host)
	c := args.String(0)
	e := args.Error(1)
	if e != nil {
		return "", e
	}
	return c, nil
}
