package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQUICResetKeyIdentity(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	binding := "udp4\x000.0.0.0:443\x000.0.0.0:443"
	first, err := quicStatelessResetKey(privateKey, "one.example:443", binding)
	require.NoError(t, err)
	// A restart reloads the same key material into a new object.
	reloaded, err := quicStatelessResetKey(append(ed25519.PrivateKey(nil), privateKey...), "one.example:443", binding)
	require.NoError(t, err)
	require.Equal(t, first, reloaded)
	other, err := quicStatelessResetKey(privateKey, "two.example:443", binding)
	require.NoError(t, err)
	require.NotEqual(t, first, other, "shared credentials must not share endpoint reset keys")
	otherBinding, err := quicStatelessResetKey(privateKey, "one.example:443", "udp6\x00[::]:443\x00[::]:443")
	require.NoError(t, err)
	require.NotEqual(t, first, otherBinding, "independent listeners must not reset each other's connections")
}
