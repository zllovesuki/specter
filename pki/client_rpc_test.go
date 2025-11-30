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

func TestRenewCertificate_Success(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	ca := generateCA(as)

	server := &Server{
		Logger:   logger,
		ClientCA: ca,
	}

	// Generate initial certificate
	clientPub, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)
	req, err := CreateRequest(clientPriv)
	as.NoError(err)

	resp, err := server.RequestCertificate(context.Background(), req)
	as.NoError(err)
	as.NotEmpty(resp.GetCertDer())

	oldCert, err := x509.ParseCertificate(resp.GetCertDer())
	as.NoError(err)
	oldCN := oldCert.Subject.CommonName

	// Generate renewal request with same key
	renewReq, err := CreateRenewalRequest(clientPriv, resp.GetCertDer())
	as.NoError(err)

	renewResp, err := server.RenewCertificate(context.Background(), renewReq)
	as.NoError(err)
	as.NotEmpty(renewResp.GetCertDer())
	as.NotEmpty(renewResp.GetCertPem())
	as.EqualValues(pki.MarshalCertificate(renewResp.GetCertDer()), renewResp.GetCertPem())

	// Verify renewed cert
	caCert, err := x509.ParseCertificate(ca.Certificate[0])
	as.NoError(err)
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)
	verify := x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	newCert, err := x509.ParseCertificate(renewResp.GetCertDer())
	as.NoError(err)

	chains, err := newCert.Verify(verify)
	as.NoError(err)
	as.Len(chains, 1)

	// Verify CN is preserved
	as.Equal(oldCN, newCert.Subject.CommonName)
	// Verify public key is the same
	as.EqualValues(clientPub, newCert.PublicKey)
}

func TestRenewCertificate_InvalidCA(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	ca := generateCA(as)
	differentCA := generateCA(as) // Different CA

	server := &Server{
		Logger:   logger,
		ClientCA: ca,
	}

	// Generate certificate with different CA
	clientPub, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	// Create cert signed by different CA
	certBytes, err := pki.GenerateCertificate(logger, differentCA, pki.IdentityRequest{
		PublicKey: clientPub,
		Subject:   pki.MakeSubjectV2(12345, []byte("test")),
	})
	as.NoError(err)

	renewReq, err := CreateRenewalRequest(clientPriv, certBytes)
	as.NoError(err)

	_, err = server.RenewCertificate(context.Background(), renewReq)
	as.Error(err)
	as.Contains(err.Error(), "permission_denied")
}

func TestRenewCertificate_BadProof(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	ca := generateCA(as)

	server := &Server{
		Logger:   logger,
		ClientCA: ca,
	}

	// Generate initial certificate
	_, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)
	req, err := CreateRequest(clientPriv)
	as.NoError(err)

	resp, err := server.RequestCertificate(context.Background(), req)
	as.NoError(err)

	// Generate PoW with DIFFERENT key
	_, differentPriv, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)
	renewReq, err := CreateRenewalRequest(differentPriv, resp.GetCertDer())
	as.NoError(err)

	_, err = server.RenewCertificate(context.Background(), renewReq)
	as.Error(err)
	as.Contains(err.Error(), "permission_denied")
}

func TestRenewCertificate_V1Unsupported(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	ca := generateCA(as)

	server := &Server{
		Logger:   logger,
		ClientCA: ca,
	}

	// Generate v1 certificate
	clientPub, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	v1Subject := pki.MakeSubjectV1(12345, "oldtoken")
	certBytes, err := pki.GenerateCertificate(logger, ca, pki.IdentityRequest{
		PublicKey: clientPub,
		Subject:   v1Subject,
	})
	as.NoError(err)

	renewReq, err := CreateRenewalRequest(clientPriv, certBytes)
	as.NoError(err)

	_, err = server.RenewCertificate(context.Background(), renewReq)
	as.Error(err)
	as.Contains(err.Error(), "failed_precondition")
	as.Contains(err.Error(), "v1 certificates cannot be renewed")
}

func TestRenewCertificate_MissingCertDer(t *testing.T) {
	as := require.New(t)
	logger := zaptest.NewLogger(t)

	ca := generateCA(as)

	server := &Server{
		Logger:   logger,
		ClientCA: ca,
	}

	_, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	renewReq, err := CreateRenewalRequest(clientPriv, nil)
	as.NoError(err)

	_, err = server.RenewCertificate(context.Background(), renewReq)
	as.Error(err)
	as.Contains(err.Error(), "current_cert_der")
}
