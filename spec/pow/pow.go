package pow

import (
	"crypto/ed25519"
	"fmt"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/util/hashcash"

	"github.com/twitchtv/twirp"
)

type Parameters struct {
	GetSubject func(pubKey ed25519.PublicKey) string
	Expires    time.Duration
	Difficulty int
}

type Decoded struct {
	PubKey  ed25519.PublicKey
	Subject string
}

func VerifySolution(req *protocol.ProofOfWork, p Parameters) (*Decoded, error) {
	if len(req.GetPubKey()) != ed25519.PublicKeySize {
		return nil, twirp.InvalidArgumentError("pub_key", "public key length is not ed25519")
	}

	if len(req.GetSignature()) != ed25519.SignatureSize {
		return nil, twirp.InvalidArgumentError("signature", "signature length is not ed25519")
	}

	if len(req.GetSolution()) == 0 {
		return nil, twirp.RequiredArgumentError("solution")
	}

	pubKey := ed25519.PublicKey(req.GetPubKey())

	if !ed25519.Verify(pubKey, []byte(req.GetSolution()), req.GetSignature()) {
		return nil, twirp.InvalidArgumentError("signature", "not a valid signature of solution by pub_key")
	}

	hc, err := hashcash.Parse(req.GetSolution())
	if err != nil {
		return nil, twirp.InvalidArgumentError("solution", err.Error())
	}

	if hc.Difficulty != p.Difficulty {
		return nil, twirp.InvalidArgumentError("solution", "solution the wrong difficulty")
	}

	if hc.ExpiresAt.IsZero() {
		return nil, twirp.InvalidArgumentError("solution", "solution cannot have zero-value expiresAt")
	}

	if time.Since(hc.ExpiresAt).Abs() > p.Expires {
		return nil, twirp.InvalidArgumentError("solution", "solution expires too far away from now")
	}

	subject := p.GetSubject(pubKey)
	if err := hc.Verify(subject); err != nil {
		return nil, twirp.InvalidArgumentError("solution", err.Error())
	}

	return &Decoded{
		PubKey:  pubKey,
		Subject: subject,
	}, nil
}

func GenerateSolution(privKey ed25519.PrivateKey, p Parameters) (*protocol.ProofOfWork, error) {
	if len(privKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("private key length is not ed25519")
	}

	var (
		pubKey  = privKey.Public().(ed25519.PublicKey)
		subject = p.GetSubject(pubKey)
	)

	hc := hashcash.New(hashcash.Hashcash{
		Subject:    subject,
		Difficulty: p.Difficulty,
		ExpiresAt:  time.Now().Add(p.Expires),
	})

	if err := hc.Solve(p.Difficulty); err != nil {
		return nil, err
	}

	sig := ed25519.Sign(privKey, []byte(hc.String()))
	return &protocol.ProofOfWork{
		PubKey:    pubKey,
		Signature: sig,
		Solution:  hc.String(),
	}, nil
}
