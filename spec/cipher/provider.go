package cipher

import (
	"crypto/tls"

	"kon.nect.sh/specter/spec/chord"
)

type CertProviderFunc func(*tls.ClientHelloInfo) (*tls.Certificate, error)

type CertProvider interface {
	Initialize(kv chord.KV) error
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}
