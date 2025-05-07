package mocks

import (
	"context"

	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
)

type KeylessService struct {
	mock.Mock
}

func (k *KeylessService) GetCertificate(ctx context.Context, req *protocol.KeylessGetCertificateRequest) (*protocol.KeylessGetCertificateResponse, error) {
	args := k.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.KeylessGetCertificateResponse), nil
}

func (k *KeylessService) Sign(ctx context.Context, req *protocol.KeylessSignRequest) (*protocol.KeylessSignResponse, error) {
	args := k.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.KeylessSignResponse), nil
}
