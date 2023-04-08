package pki

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
)

func UnmarshalPrivateKey(pemBytes []byte) (ed25519.PrivateKey, error) {
	p, _ := pem.Decode(pemBytes)
	key, err := x509.ParsePKCS8PrivateKey(p.Bytes)
	if err != nil {
		return nil, err
	}
	if ed, ok := key.(ed25519.PrivateKey); ok {
		return ed, nil
	}
	return nil, x509.ErrUnsupportedAlgorithm
}

func MarshalCertificate(derBytes []byte) (pemBytes []byte) {
	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})
	return certPEM.Bytes()
}
