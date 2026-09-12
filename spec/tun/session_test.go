package tun

import (
	"strings"
	"testing"

	"go.miragespace.co/specter/spec/chord"

	"github.com/stretchr/testify/require"
)

func TestLabelAndAliasFormats(t *testing.T) {
	label, inv, err := EphemeralLabel(chord.MaxIdentitifer-1, []byte("public key"))
	require.NoError(t, err)
	require.Len(t, label, 47)
	home, parsed, ok := ParseEphemeralLabel(label)
	require.True(t, ok)
	require.Equal(t, chord.MaxIdentitifer-1, home)
	require.Equal(t, inv, parsed)
	_, _, err = EphemeralLabel(chord.MaxIdentitifer, nil)
	require.Error(t, err)
	for _, bad := range []string{strings.ToUpper(label), label[:46], label + ".example.com", label[:46] + "g"} {
		_, _, ok := ParseEphemeralLabel(bad)
		require.False(t, ok, bad)
	}
	alias := SessionAlias(inv)
	require.Len(t, alias, 40)
	require.True(t, IsSessionAlias(alias))
	for _, bad := range []string{strings.ToUpper(alias), alias + "0", "v2:" + strings.Repeat("a", 46), strings.Repeat("a", 44)} {
		require.False(t, IsSessionAlias(bad))
	}
	secret := [32]byte{1, 2, 3, 4}
	token := EncodeDelegationToken(secret)
	require.Len(t, token, 47)
	got, err := ParseDelegationToken(token)
	require.NoError(t, err)
	require.Equal(t, secret, got)
	id := DelegationID(secret)
	require.True(t, IsDelegationID(id))
	require.False(t, IsDelegationID(strings.ToUpper(id)))
	for _, bad := range []string{id, token + "=", " " + token, token[:46], token[:46] + "B"} {
		_, err := ParseDelegationToken(bad)
		require.Error(t, err, bad)
	}
}
