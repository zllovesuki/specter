package cipher

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
)

var (
	// support only h3 (drop support for h3-29)
	H3Protos = []string{"h3"}
)

// we will require the use of ECDSA certificates for Chord
func GetPeerTLSConfig(ca *x509.CertPool, node tls.Certificate, protos []string) *tls.Config {
	return &tls.Config{
		Rand:         rand.Reader,
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
			tls.CurveP256,
		},
	}
}

// our acme cert generation uses ECDSA (P-256), thus we will skip
// ciphers that do not do elliptic curve DH
func GetGatewayTLSConfig(provider CertProviderFunc, protos []string) *tls.Config {
	return &tls.Config{
		Rand:           rand.Reader,
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

// generate *tls.Config that will work for both HTTP3 (h3) and specter TCP tunnel (tcp).
// Standard expects h3 or h3-29 only in protos exchange, otherwise alt-svc will break
func GetGatewayHTTP3Config(provider CertProviderFunc, protos []string) *tls.Config {
	baseCfg := GetGatewayTLSConfig(provider, nil)
	baseCfg.GetConfigForClient = func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
		var isH3 bool
		var isSpecter bool
		var xCfg *tls.Config

		for _, cP := range chi.SupportedProtos {
			for _, sP := range protos {
				if sP == cP {
					isSpecter = true
				}
			}
			for _, hP := range H3Protos {
				if hP == cP {
					isH3 = true
				}
			}
		}
		if isH3 || isSpecter {
			xCfg = baseCfg.Clone()
			xCfg.NextProtos = chi.SupportedProtos
			return xCfg, nil
		}
		return nil, errors.New("cipher: no mutually supported protocols")
	}
	return baseCfg
}
