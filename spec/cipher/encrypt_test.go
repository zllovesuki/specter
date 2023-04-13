package cipher

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/chacha20poly1305"
	"kon.nect.sh/specter/spec/protocol"
)

func TestAESEncryptDecrypt(t *testing.T) {
	as := require.New(t)

	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	kvKey := []byte("/key/sub/A")
	kvVal := []byte("the quick brown fox jumps over the lazy dog")

	salt, encKey, err := DeriveEncryptionKey(privKey, kvKey, 16) // AES-128 has 16 bytes key
	as.NoError(err)

	// do encryption
	encBlock, err := aes.NewCipher(encKey)
	as.NoError(err)

	encGcm, err := cipher.NewGCM(encBlock)
	as.NoError(err)

	nonce := make([]byte, encGcm.NonceSize())
	_, err = rand.Read(nonce)
	as.NoError(err)

	cipherText := encGcm.Seal(nil, nonce, kvVal, nil)

	y := &protocol.CipherText{
		Cipher:     protocol.Cipher_AES_128_GCM,
		HkdfSalt:   salt,
		AeadNonce:  nonce,
		CipherText: cipherText,
	}

	buf, err := y.MarshalVT()
	as.NoError(err)

	t.Logf("plaintext length:  %v\n", len(kvVal))
	t.Logf("serialized length: %+v\n", len(buf))

	// do decryption
	decBlock, err := aes.NewCipher(encKey)
	as.NoError(err)

	decGcm, err := cipher.NewGCM(decBlock)
	as.NoError(err)

	plaintext, err := decGcm.Open(nil, nonce, cipherText, nil)
	as.NoError(err)

	as.EqualValues(kvVal, plaintext)
}

func TestChacha20EncryptDecrypt(t *testing.T) {
	as := require.New(t)

	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	as.NoError(err)

	kvKey := []byte("/key/sub/A")
	kvVal := []byte("the quick brown fox jumps over the lazy dog")

	salt, encKey, err := DeriveEncryptionKey(privKey, kvKey, chacha20poly1305.KeySize)
	as.NoError(err)

	// do encryption
	encCC, err := chacha20poly1305.New(encKey)
	as.NoError(err)

	nonce := make([]byte, encCC.NonceSize())
	_, err = rand.Read(nonce)
	as.NoError(err)

	cipherText := encCC.Seal(nil, nonce, kvVal, nil)

	y := &protocol.CipherText{
		Cipher:     protocol.Cipher_CHACHA20_POLY1305,
		HkdfSalt:   salt,
		AeadNonce:  nonce,
		CipherText: cipherText,
	}

	buf, err := y.MarshalVT()
	as.NoError(err)

	t.Logf("plaintext length:  %v\n", len(kvVal))
	t.Logf("serialized length: %+v\n", len(buf))

	// do decryption
	decCC, err := chacha20poly1305.New(encKey)
	as.NoError(err)

	plaintext, err := decCC.Open(nil, nonce, cipherText, nil)
	as.NoError(err)

	as.EqualValues(kvVal, plaintext)
}
