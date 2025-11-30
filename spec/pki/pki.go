package pki

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"go.uber.org/zap"
)

const (
	HashcashDifficulty int           = 18
	HashcashExpires    time.Duration = time.Second * 10

	// DefaultCertValidity is the default validity duration for client certificates.
	DefaultCertValidity time.Duration = time.Hour * 24 * 365 * 5 // 5 years
)

type IdentityRequest struct {
	PublicKey []byte
	Subject   pkix.Name

	// ValidFor specifies the certificate validity duration.
	// If zero, defaults to DefaultCertValidity.
	ValidFor time.Duration
}

func GenerateCertificate(logger *zap.Logger, ca tls.Certificate, req IdentityRequest) (derBytes []byte, err error) {
	if len(req.PublicKey) != ed25519.PublicKeySize {
		err = fmt.Errorf("pki: public key is not ed25519")
		return
	}

	caCert, err := x509.ParseCertificate(ca.Certificate[0])
	if err != nil {
		err = fmt.Errorf("pki: failed to parse client ca: %w", err)
		return
	}

	sn, err := rand.Int(rand.Reader, max)
	if err != nil {
		err = fmt.Errorf("pki: failed to generate certificate serial: %w", err)
		return
	}

	now := time.Now()
	validFor := DefaultCertValidity
	if req.ValidFor > 0 {
		validFor = req.ValidFor
	}
	notAfter := now.Add(validFor)

	cert := &x509.Certificate{
		SerialNumber:          sn,
		Subject:               req.Subject,
		NotBefore:             now,
		NotAfter:              notAfter,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, caCert, ed25519.PublicKey(req.PublicKey), ca.PrivateKey)
	if err != nil {
		err = fmt.Errorf("pki: failed to generate client certificate: %w", err)
		return
	}

	logger.Info("New client certificate issued", zap.String("commonName", req.Subject.CommonName))

	return certBytes, nil
}

func GeneratePrivKey() (privKey ed25519.PublicKey, keyPem string) {
	certPubKey, certPrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	x509PrivKey, err := x509.MarshalPKCS8PrivateKey(certPrivKey)
	if err != nil {
		panic(err)
	}

	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509PrivKey,
	})

	return certPubKey, certPrivKeyPEM.String()
}

var (
	max = new(big.Int)
)

func init() {
	max.Exp(big.NewInt(2), big.NewInt(130), nil).Sub(max, big.NewInt(1))
}
