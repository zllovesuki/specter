package pki

import (
	"bytes"
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
		return nil, err
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

func (p *Server) RenewCertificate(ctx context.Context, req *protocol.CertificateRenewalRequest) (*protocol.CertificateResponse, error) {
	// Validate current_cert_der is provided
	if len(req.GetCurrentCertDer()) == 0 {
		return nil, twirp.RequiredArgumentError("current_cert_der")
	}

	// Parse the current certificate
	oldCert, err := x509.ParseCertificate(req.GetCurrentCertDer())
	if err != nil {
		return nil, twirp.InvalidArgumentError("current_cert_der", "failed to parse certificate")
	}

	// Verify certificate is from our Client CA
	caCert, err := x509.ParseCertificate(p.ClientCA.Certificate[0])
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}
	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	_, err = oldCert.Verify(x509.VerifyOptions{
		Roots:     caPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return nil, twirp.PermissionDenied.Error("certificate not issued by this CA")
	}

	// Extract identity and check version
	identity, err := pki.ExtractCertificateIdentity(oldCert)
	if err != nil {
		return nil, twirp.InvalidArgumentError("current_cert_der", "failed to extract identity from certificate")
	}

	// Reject v1 certificates - they must use manual migration
	if identity.Version == pki.TokenV1 {
		return nil, twirp.FailedPrecondition.Error("v1 certificates cannot be renewed; please use the migration tool (util/migrator) to upgrade to v2")
	}

	// Verify PoW with same parameters as initial issuance
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
		return nil, err
	}

	// Verify PoW public key matches the certificate's public key
	oldPubKey, ok := oldCert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return nil, twirp.InvalidArgumentError("current_cert_der", "certificate does not contain an ed25519 public key")
	}
	if !bytes.Equal(d.PubKey, oldPubKey) {
		return nil, twirp.PermissionDenied.Error("proof does not match current certificate key")
	}

	// Issue renewed certificate with the same subject (preserves CN/identity)
	certBytes, err := pki.GenerateCertificate(p.Logger, p.ClientCA, pki.IdentityRequest{
		PublicKey: d.PubKey,
		Subject:   oldCert.Subject, // Preserve exact subject from old certificate
	})
	if err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	certPem := pki.MarshalCertificate(certBytes)

	p.Logger.Info("Certificate renewed", zap.Object("identity", identity))

	return &protocol.CertificateResponse{
		CertDer: certBytes,
		CertPem: certPem,
	}, nil
}
