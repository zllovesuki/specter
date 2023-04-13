package cipher

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"

	"golang.org/x/crypto/hkdf"
)

func DeriveEncryptionKey(privKey ed25519.PrivateKey, info []byte, size int) (salt, encKey []byte, err error) {
	seed := privKey[:32]

	hash := sha256.New
	salt = make([]byte, hash().Size())
	_, err = rand.Read(salt)
	if err != nil {
		return
	}

	gen := hkdf.New(hash, seed, salt, info)

	encKey = make([]byte, size)
	_, err = gen.Read(encKey)
	if err != nil {
		return
	}

	return
}
