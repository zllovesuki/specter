package pki

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/pki"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
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

func TestSigning(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	cert := generateCA(as)

	server := &Server{
		Logger:   logger,
		ClientCA: cert,
	}

	clientPub, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)
	req, err := CreateRequest(clientPriv)
	as.NoError(err)

	resp, err := server.RequestCertificate(context.Background(), req)
	as.NoError(err)
	as.NotEmpty(resp.GetCertDer())
	as.NotEmpty(resp.GetCertPem())
	as.EqualValues(pki.MarshalCertificate(resp.GetCertDer()), resp.GetCertPem())

	ca, err := x509.ParseCertificate(cert.Certificate[0])
	as.NoError(err)
	caPool := x509.NewCertPool()
	caPool.AddCert(ca)
	verify := x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCert, err := x509.ParseCertificate(resp.GetCertDer())
	as.NoError(err)

	chains, err := clientCert.Verify(verify)
	as.NoError(err)
	c := chains[0][0]
	as.EqualValues(clientPub, c.PublicKey)
}
