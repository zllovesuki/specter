package tun

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const DelegationTokenPrefix = "tg1_"

const DelegationRecordVersion = 1

const MaxDelegationsPerOwner = 128

func EncodeDelegationToken(secret [32]byte) string {
	return DelegationTokenPrefix + base64.RawURLEncoding.EncodeToString(secret[:])
}

func ParseDelegationToken(token string) (secret [32]byte, err error) {
	if len(token) != 47 || !strings.HasPrefix(token, DelegationTokenPrefix) {
		return secret, fmt.Errorf("invalid delegation token")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token[4:])
	if err != nil || len(raw) != len(secret) {
		return secret, fmt.Errorf("invalid delegation token")
	}
	copy(secret[:], raw)
	return secret, nil
}

func DelegationID(secret [32]byte) string {
	digest := sha256.Sum256(secret[:])
	return hex.EncodeToString(digest[:])
}

func IsDelegationID(id string) bool { return len(id) == 64 && lowerHex(id) }
