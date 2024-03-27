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

	"go.miragespace.co/specter/spec/pki"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func generateCA(as *require.Assertions) (tls.Certificate, *x509.CertPool) {
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

	cert, err := x509.ParseCertificate(caBytes)
	as.NoError(err)

	pool := x509.NewCertPool()
	pool.AddCert(cert)

	return tls.Certificate{
		Certificate: [][]byte{caBytes},
		PrivateKey:  caPrivKey,
	}, pool
}

func TestSigning(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	cert, pool := generateCA(as)

	server := &Server{
		Logger:   logger,
		ClientCA: cert,
		CAPool:   pool,
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

	verify := x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCert, err := x509.ParseCertificate(resp.GetCertDer())
	as.NoError(err)

	chains, err := clientCert.Verify(verify)
	as.NoError(err)
	c := chains[0][0]
	as.EqualValues(clientPub, c.PublicKey)
}

func TestRenewal(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	cert, pool := generateCA(as)

	server := &Server{
		Logger:   logger,
		ClientCA: cert,
		CAPool:   pool,
	}

	verify := x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
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

	prevCert, err := x509.ParseCertificate(resp.GetCertDer())
	as.NoError(err)
	chains, err := prevCert.Verify(verify)
	as.NoError(err)
	c1 := chains[0][0]
	as.EqualValues(clientPub, c1.PublicKey)

	logger.Info("Previous certificate",
		zap.Stringer("serial", prevCert.SerialNumber),
	)

	rreq, err := CreateRenewalRequest(clientPriv, resp.GetCertDer())
	as.NoError(err)
	resp, err = server.RenewCertificate(context.Background(), rreq)
	as.NoError(err)
	as.NotEmpty(resp.GetCertDer())
	as.NotEmpty(resp.GetCertPem())
	as.EqualValues(pki.MarshalCertificate(resp.GetCertDer()), resp.GetCertPem())

	newCert, err := x509.ParseCertificate(resp.GetCertDer())
	as.NoError(err)
	chains, err = newCert.Verify(verify)
	as.NoError(err)
	c2 := chains[0][0]
	as.EqualValues(clientPub, c2.PublicKey)

	logger.Info("Renewed certificate",
		zap.Stringer("serial", newCert.SerialNumber),
	)

	as.NotEqual(0, prevCert.SerialNumber.Cmp(newCert.SerialNumber))
}
