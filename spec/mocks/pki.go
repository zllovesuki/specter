//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"context"

	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
)

type PKIClient struct {
	mock.Mock
}

func (p *PKIClient) RequestCertificate(ctx context.Context, req *protocol.CertificateRequest) (*protocol.CertificateResponse, error) {
	args := p.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.CertificateResponse), nil
}

var _ protocol.PKIService = (*PKIClient)(nil)
