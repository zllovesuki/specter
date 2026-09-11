package main

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPeerCertificateSerial(t *testing.T) {
	valid := &x509.Certificate{
		DNSNames:     []string{"dev.con.nect.sh"},
		SerialNumber: big.NewInt(42),
	}
	wrongHost := &x509.Certificate{
		Subject:      pkix.Name{CommonName: "dev.con.nect.sh"},
		DNSNames:     []string{"other.example.com"},
		SerialNumber: big.NewInt(43),
	}
	for _, tc := range []struct {
		name  string
		chain []*x509.Certificate
		valid bool
	}{
		{name: "SAN without Common Name", chain: []*x509.Certificate{valid}, valid: true},
		{name: "missing certificate"},
		{name: "wrong SAN with matching Common Name", chain: []*x509.Certificate{wrongHost}},
		{name: "matching intermediate cannot replace leaf", chain: []*x509.Certificate{wrongHost, valid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serial, err := peerCertificateSerial(tls.ConnectionState{PeerCertificates: tc.chain}, "dev.con.nect.sh")
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, "42", serial)
			} else {
				require.Error(t, err)
				require.Empty(t, serial)
			}
		})
	}
}
