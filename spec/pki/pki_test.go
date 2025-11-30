package pki

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	testCommonName = "test cn"
)

func generateCA(as *require.Assertions) tls.Certificate {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1234),
		Subject: pkix.Name{
			CommonName: "test ca",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPubKey, caPrivKey, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, caPubKey, caPrivKey)
	as.NoError(err)

	return tls.Certificate{
		Certificate: [][]byte{caBytes},
		PrivateKey:  caPrivKey,
	}
}

func TestGenerateCertificate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	as := require.New(t)
	pubKey, _ := GeneratePrivKey()

	ca := generateCA(as)
	caCert, err := x509.ParseCertificate(ca.Certificate[0])
	as.NoError(err)

	der, err := GenerateCertificate(logger, ca, IdentityRequest{
		PublicKey: pubKey,
		Subject: pkix.Name{
			CommonName: testCommonName,
		},
	})
	as.NoError(err)

	cert, err := x509.ParseCertificate(der)
	as.NoError(err)
	as.Equal(testCommonName, cert.Subject.CommonName)

	correctCa := x509.NewCertPool()
	correctCa.AddCert(caCert)

	_, err = cert.Verify(x509.VerifyOptions{
		Roots: correctCa,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	})
	as.NoError(err)

	ca2 := generateCA(as)
	caCert2, err := x509.ParseCertificate(ca2.Certificate[0])
	as.NoError(err)

	wrongCa := x509.NewCertPool()
	wrongCa.AddCert(caCert2)

	_, err = cert.Verify(x509.VerifyOptions{
		Roots: wrongCa,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	})
	as.Error(err)
}

func TestGenerateCertificate_CustomValidity(t *testing.T) {
	logger := zaptest.NewLogger(t)
	as := require.New(t)
	pubKey, _ := GeneratePrivKey()

	ca := generateCA(as)

	customValidity := time.Hour * 2

	der, err := GenerateCertificate(logger, ca, IdentityRequest{
		PublicKey: pubKey,
		Subject: pkix.Name{
			CommonName: "custom-validity-test",
		},
		ValidFor: customValidity,
	})
	as.NoError(err)

	cert, err := x509.ParseCertificate(der)
	as.NoError(err)

	// Verify the certificate validity is approximately the custom duration
	// Allow 1 minute tolerance for test execution time
	actualValidity := cert.NotAfter.Sub(cert.NotBefore)
	as.InDelta(customValidity.Seconds(), actualValidity.Seconds(), 60)
}

func TestGenerateCertificate_DefaultValidity(t *testing.T) {
	logger := zaptest.NewLogger(t)
	as := require.New(t)
	pubKey, _ := GeneratePrivKey()

	ca := generateCA(as)

	der, err := GenerateCertificate(logger, ca, IdentityRequest{
		PublicKey: pubKey,
		Subject: pkix.Name{
			CommonName: "default-validity-test",
		},
		// ValidFor not set, should default to DefaultCertValidity
	})
	as.NoError(err)

	cert, err := x509.ParseCertificate(der)
	as.NoError(err)

	// Verify the certificate validity matches the default constant
	actualValidity := cert.NotAfter.Sub(cert.NotBefore)
	// Allow 1 minute tolerance for test execution time
	as.InDelta(DefaultCertValidity.Seconds(), actualValidity.Seconds(), 60)
}

func TestPrivateKey(t *testing.T) {
	as := require.New(t)
	pubKey, keyPem := GeneratePrivKey()
	un, err := UnmarshalPrivateKey([]byte(keyPem))
	as.NoError(err)

	privKey := ed25519.PrivateKey(un)
	as.EqualValues(pubKey, privKey.Public().(ed25519.PublicKey))
}
