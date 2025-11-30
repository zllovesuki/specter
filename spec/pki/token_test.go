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
	as.Equal(TokenV1, identity.Version)
	as.Equal(id, identity.ID)
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
	as.Equal(TokenV2, identity.Version)
	as.Equal(id, identity.ID)
}

func TestNodeIdentity(t *testing.T) {
	as := require.New(t)

	// Test that NodeIdentity produces expected protocol.Node for v1
	v1Identity := &Identity{
		ID:      12345,
		Token:   []byte("oldtoken"),
		Version: TokenV1,
	}
	v1Node := v1Identity.NodeIdentity()
	as.Equal(uint64(12345), v1Node.GetId())
	as.Equal("oldtoken", v1Node.GetAddress())
	as.True(v1Node.GetRendezvous())

	// Test that NodeIdentity produces expected protocol.Node for v2
	// For v2, Token is the full CN
	v2CN := "v2:67890:AQIDBAU="
	v2Identity := &Identity{
		ID:      67890,
		Token:   []byte(v2CN),
		Version: TokenV2,
	}
	v2Node := v2Identity.NodeIdentity()
	as.Equal(uint64(67890), v2Node.GetId())
	as.Equal(v2CN, v2Node.GetAddress())
	as.True(v2Node.GetRendezvous())
}
