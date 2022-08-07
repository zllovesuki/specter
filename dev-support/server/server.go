package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/cipher"

	"github.com/caddyserver/certmagic"
	"github.com/mholt/acmez/acme"
	"go.uber.org/zap"
)

func IsDev(ca string) bool {
	switch ca {
	case certmagic.LetsEncryptProductionCA:
		return false
	default:
		return true
	}
}

type NoopSolver struct {
	Logger *zap.Logger
}

func (s NoopSolver) Present(ctx context.Context, chal acme.Challenge) error {
	s.Logger.Debug("solver present challenge", zap.Any("challenge", chal))
	return nil
}

func (s NoopSolver) CleanUp(ctx context.Context, chal acme.Challenge) error {
	s.Logger.Debug("solver cleanup challenge", zap.Any("challenge", chal))
	return nil
}

type SelfSignedProvider struct {
	cert       *tls.Certificate
	RootDomain string
}

var _ cipher.CertProvider = (*SelfSignedProvider)(nil)

func (s *SelfSignedProvider) Initialize(node chord.KV) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		panic(err)
	}
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Dev"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames: []string{
			s.RootDomain,
			"*." + s.RootDomain,
		},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}

	s.cert = &tlsCert
}

func (s *SelfSignedProvider) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return s.cert, nil
}
