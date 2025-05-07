package mocks

import (
	"context"
	"crypto/tls"

	"go.miragespace.co/specter/spec/cipher"

	"github.com/stretchr/testify/mock"
)

type CertProvider struct {
	mock.Mock
}

func (c *CertProvider) GetCertificate(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
	args := c.Called(chi)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*tls.Certificate), nil
}

func (c *CertProvider) GetCertificateWithContext(ctx context.Context, chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
	args := c.Called(ctx, chi)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*tls.Certificate), nil
}

func (c *CertProvider) Initialize(ctx context.Context) error {
	panic("unimplemented")
}

func (c *CertProvider) OnHandshake(cipher.OnHandshakeFunc) {
	panic("unimplemented")
}

var _ cipher.CertProvider = (*CertProvider)(nil)
