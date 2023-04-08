package pki

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/util/hashcash"
)

func CreateRequest(privKey ed25519.PrivateKey) (*protocol.CertificateRequest, error) {
	pubKey := privKey.Public().(ed25519.PublicKey)

	h := sha256.New()
	h.Write(pubKey)
	subject := h.Sum(nil)

	hc := hashcash.New(hashcash.Hashcash{
		Subject:    base64.URLEncoding.EncodeToString(subject),
		Difficulty: pki.HashcashDifficulty,
		ExpiresAt:  time.Now().Add(pki.HashcashExpires),
	})

	if err := hc.Solve(pki.HashcashDifficulty); err != nil {
		return nil, err
	}

	sig := ed25519.Sign(privKey, []byte(hc.String()))

	return &protocol.CertificateRequest{
		PubKey:           pubKey,
		Signature:        sig,
		HashcashSolution: hc.String(),
	}, nil
}
