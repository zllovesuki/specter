package pki

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func makeCert(as *require.Assertions, sub pkix.Name) *x509.Certificate {
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1234),
		Subject:      sub,
	}

	caPubKey, caPrivKey, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, caPubKey, caPrivKey)
	as.NoError(err)

	parsed, err := x509.ParseCertificate(certBytes)
	as.NoError(err)

	return parsed
}

func TestTokenV1(t *testing.T) {
	as := require.New(t)

	var (
		id    uint64 = 256
		token string = "token"
	)

	sub := MakeSubjectV1(id, token)
	t.Log(sub)
	identity, err := ExtractCertificateIdentity(makeCert(as, sub))
	as.NoError(err)
	as.Contains(sub.CommonName, string(identity.Token))
}

func TestTokenV2(t *testing.T) {
	as := require.New(t)

	var (
		id   uint64 = 256
		hash []byte = []byte{1, 2, 3, 4, 5}
	)

	sub := MakeSubjectV2(id, hash)
	t.Log(sub)
	identity, err := ExtractCertificateIdentity(makeCert(as, sub))
	as.NoError(err)
	as.Equal(sub.CommonName, string(identity.Token))
}
