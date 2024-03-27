package pki

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"

	"go.miragespace.co/specter/spec/pki"
	"go.miragespace.co/specter/spec/pow"
	"go.miragespace.co/specter/spec/protocol"
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

func CreateRenewalRequest(privKey ed25519.PrivateKey, prevCertDer []byte) (*protocol.RenewalRequest, error) {
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

	return &protocol.RenewalRequest{
		Proof:       proof,
		PrevCertDer: prevCertDer,
	}, nil
}
