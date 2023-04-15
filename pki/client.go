package pki

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"

	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/spec/pow"
	"kon.nect.sh/specter/spec/protocol"
)

func CreateRequest(privKey ed25519.PrivateKey) (*protocol.CertificateRequest, error) {
	proof, err := pow.GenerateSolution(privKey, pow.Parameters{
		Difficulty: pki.HashcashDifficulty,
		Expires:    pki.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			h := sha256.New()
			h.Write(pubKey)
			return base64.URLEncoding.EncodeToString(h.Sum(nil))
		},
	})
	if err != nil {
		return nil, err
	}

	return &protocol.CertificateRequest{
		Proof: proof,
	}, nil
}
