package tun

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"go.miragespace.co/specter/spec/chord"
)

const SessionAliasPrefix = "session:"

const EphemeralLabelPrefix = "e1-"

func lowerHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func IsSessionAlias(addr string) bool {
	return len(addr) == 40 && strings.HasPrefix(addr, SessionAliasPrefix) && lowerHex(addr[len(SessionAliasPrefix):])
}

func SessionAlias(inv [16]byte) string { return SessionAliasPrefix + hex.EncodeToString(inv[:]) }

func EphemeralLabel(home uint64, spki []byte) (label string, inv [16]byte, err error) {
	if home >= chord.MaxIdentitifer {
		return "", inv, fmt.Errorf("home ID exceeds 48 bits")
	}
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], home)
	h := sha256.New()
	h.Write([]byte("specter/ephemeral/v1\x00"))
	h.Write(encoded[2:])
	h.Write(spki)
	copy(inv[:], h.Sum(nil))
	return EphemeralLabelPrefix + hex.EncodeToString(encoded[2:]) + hex.EncodeToString(inv[:]), inv, nil
}

func ParseEphemeralLabel(label string) (home uint64, inv [16]byte, ok bool) {
	if len(label) != 47 || !strings.HasPrefix(label, EphemeralLabelPrefix) || !lowerHex(label[3:]) {
		return
	}
	raw, _ := hex.DecodeString(label[3:])
	for _, b := range raw[:6] {
		home = home<<8 | uint64(b)
	}
	copy(inv[:], raw[6:])
	return home, inv, true
}
