package pki

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/util/hashcash"

	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

type Server struct {
	Logger   *zap.Logger
	ClientCA tls.Certificate
}

var _ protocol.PKIService = (*Server)(nil)

func (p *Server) RequestCertificate(ctx context.Context, req *protocol.CertificateRequest) (*protocol.CertificateResponse, error) {
	if len(req.GetPubKey()) != ed25519.PublicKeySize {
		return nil, twirp.InvalidArgumentError("pub_key", "public key length is not ed25519")
	}

	if len(req.GetSignature()) != ed25519.SignatureSize {
		return nil, twirp.InvalidArgumentError("signature", "signature length is not ed25519")
	}

	if len(req.GetHashcashSolution()) == 0 {
		return nil, twirp.RequiredArgumentError("hashcash_solution")
	}

	if !ed25519.Verify(ed25519.PublicKey(req.GetPubKey()), []byte(req.GetHashcashSolution()), req.GetSignature()) {
		return nil, twirp.InvalidArgumentError("signature", "not a valid signature of hashcash_solution by pub_key")
	}

	hc, err := hashcash.Parse(req.GetHashcashSolution())
	if err != nil {
		return nil, twirp.InvalidArgumentError("hashcash_solution", err.Error())
	}

	if hc.Difficulty < pki.HashcashDifficulty {
		return nil, twirp.InvalidArgumentError("hashcash_solution", "hashcash difficulty is too low")
	}

	if hc.ExpiresAt.IsZero() {
		return nil, twirp.InvalidArgumentError("hashcash_solution", "hashcash cannot have zero-value expiresAt")
	}

	if time.Since(hc.ExpiresAt).Abs() > pki.HashcashExpires {
		return nil, twirp.InvalidArgumentError("hashcash_solution", "hashcash expires too far away from now")
	}

	if hc.Difficulty != pki.HashcashDifficulty {
		return nil, twirp.InvalidArgumentError("hashcash_solution", "hashcash has the wrong difficulty")
	}

	h := sha256.New()
	h.Write(req.GetPubKey())
	subject := h.Sum(nil)

	if err := hc.Verify(base64.URLEncoding.EncodeToString(subject)); err != nil {
		return nil, twirp.InvalidArgumentError("hashcash_solution", err.Error())
	}

	id := chord.Random()
	certSubject := pki.MakeSubjectV2(id, subject)

	certBytes, err := pki.GenerateCertificate(p.Logger, p.ClientCA, pki.IdentityRequest{
		PublicKey: req.GetPubKey(),
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
