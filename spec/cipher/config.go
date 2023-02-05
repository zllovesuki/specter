package cipher

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/quic-go/quic-go/http3"
)

var (
	H3Protos = []string{http3.NextProtoH3, http3.NextProtoH3Draft29}
)

// we will require the use of ECDSA certificates for Chord
func GetPeerTLSConfig(ca *x509.CertPool, node tls.Certificate, protos []string) *tls.Config {
	return &tls.Config{
		RootCAs:      ca,
		ClientCAs:    ca,
		Certificates: []tls.Certificate{node},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		NextProtos:   protos,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			// TLS 1.3 ciphers
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
		CurvePreferences: []tls.CurveID{
			tls.X25519, tls.CurveP256,
		},
	}
}

// our acme cert generation uses ECDSA (P-256), thus we will skip
// ciphers that do not do elliptic curve DH
func GetGatewayTLSConfig(provider CertProviderFunc, protos []string) *tls.Config {
	return &tls.Config{
		GetCertificate: provider,
		ClientAuth:     tls.NoClientCert,
		NextProtos:     protos,
		MinVersion:     tls.VersionTLS12,
		// https://wiki.mozilla.org/Security/Server_Side_TLS (Intermediate compatibility)
		CipherSuites: []uint16{
			// TLS 1.3 ciphers
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			// TLS 1.2 ciphers
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},
		CurvePreferences: []tls.CurveID{
			tls.X25519, tls.CurveP256, tls.CurveP384,
		},
	}
}
