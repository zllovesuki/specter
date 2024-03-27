package pki

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/pow"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

type Server struct {
	Logger   *zap.Logger
	ClientCA tls.Certificate
	CAPool   *x509.CertPool
}

var _ protocol.PKIService = (*Server)(nil)

func (p *Server) RequestCertificate(ctx context.Context, req *protocol.CertificateRequest) (*protocol.CertificateResponse, error) {
	var hashed []byte

	d, err := pow.VerifySolution(req.GetProof(), pow.Parameters{
		Difficulty: pki.HashcashDifficulty,
		Expires:    pki.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			h := sha256.New()
			h.Write(pubKey)
			hashed = h.Sum(nil)
			return base64.URLEncoding.EncodeToString(hashed)
		},
	})
	if err != nil {
		return nil, twirp.InvalidArgumentError("proof", err.Error())
	}

	id := chord.Random()
	certSubject := pki.MakeSubjectV2(id, hashed)

	certBytes, err := pki.GenerateCertificate(p.Logger, p.ClientCA, pki.IdentityRequest{
		PublicKey: d.PubKey,
		Subject:   certSubject,
	})
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	certPem := pki.MarshalCertificate(certBytes)

	return &protocol.CertificateResponse{
		CertDer: certBytes,
		CertPem: certPem,
	}, nil
}

func (p *Server) RenewCertificate(ctx context.Context, req *protocol.RenewalRequest) (*protocol.CertificateResponse, error) {
	var hashed []byte

	d, err := pow.VerifySolution(req.GetProof(), pow.Parameters{
		Difficulty: pki.HashcashDifficulty,
		Expires:    pki.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			h := sha256.New()
			h.Write(pubKey)
			hashed = h.Sum(nil)
			return base64.URLEncoding.EncodeToString(hashed)
		},
	})
	if err != nil {
		return nil, twirp.InvalidArgumentError("proof", err.Error())
	}

	prevDer := req.GetPrevCertDer()
	prevCert, err := x509.ParseCertificate(prevDer)
	if err != nil {
		return nil, twirp.InvalidArgumentError("prevCertDer", err.Error())
	}

	chains, err := prevCert.Verify(x509.VerifyOptions{
		Roots:     p.CAPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return nil, twirp.InvalidArgumentError("prevCertDer", err.Error())
	}

	verified := chains[0][0]
	switch verified.PublicKeyAlgorithm {
	case x509.Ed25519:
		pubKey := verified.PublicKey.(ed25519.PublicKey)
		if !pubKey.Equal(d.PubKey) {
			return nil, twirp.InvalidArgumentError("prevCertDer", "public key in certificate does not match the public key in PoW")
		}
	default:
		return nil, twirp.InvalidArgumentError("prevCertDer", "an invalid certificate was provided")
	}

	certBytes, err := pki.GenerateCertificate(p.Logger, p.ClientCA, pki.IdentityRequest{
		PublicKey: d.PubKey,
		Subject:   verified.Subject,
	})
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	certPem := pki.MarshalCertificate(certBytes)

	return &protocol.CertificateResponse{
		CertDer: certBytes,
		CertPem: certPem,
	}, nil
}
