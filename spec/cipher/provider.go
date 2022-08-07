package cipher

import (
	"crypto/tls"

	"kon.nect.sh/specter/spec/chord"
)

type CertProviderFunc func(*tls.ClientHelloInfo) (*tls.Certificate, error)

// ideally we want to storage certificates within the chord DHT, but I need to solve
// the issues of non-atomic access
type CertProvider interface {
	Initialize(node chord.KV)
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}
