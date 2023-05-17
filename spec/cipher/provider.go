package cipher

import (
	"context"
	"crypto/tls"
)

type CertProviderFunc func(*tls.ClientHelloInfo) (*tls.Certificate, error)

type OnHandshakeFunc func(sni string)

type CertProvider interface {
	Initialize(ctx context.Context) error
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
	OnHandshake(OnHandshakeFunc)
}
