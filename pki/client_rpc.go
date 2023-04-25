package pki

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/spec/pow"
	"kon.nect.sh/specter/spec/protocol"

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
