package listen

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAddressesUsesBaseWhenOverridesEmpty(t *testing.T) {
	addrs, err := ParseAddresses("udp", "0.0.0.0:53", nil)
	require.NoError(t, err)
	require.Len(t, addrs, 1)

	addr := addrs[0]
	require.Equal(t, "0.0.0.0:53", addr.Address)
	require.Equal(t, "0.0.0.0", addr.Host)
	require.Equal(t, "udp4", addr.Network)
	require.Equal(t, IPV4, addr.Version)
}

func TestParseAddressesDedupAndTrims(t *testing.T) {
	addrs, err := ParseAddresses("tcp", "127.0.0.1:80", []string{" 127.0.0.1:80 ", "[::1]:80", "127.0.0.1:80"})
	require.NoError(t, err)
	require.Len(t, addrs, 2)

	require.Equal(t, "127.0.0.1:80", addrs[0].Address)
	require.Equal(t, "tcp4", addrs[0].Network)
	require.Equal(t, IPV4, addrs[0].Version)

	require.Equal(t, "[::1]:80", addrs[1].Address)
	require.Equal(t, "tcp6", addrs[1].Network)
	require.Equal(t, IPV6, addrs[1].Version)
}

func TestParseAddressesInvalid(t *testing.T) {
	_, err := ParseAddresses("tcp", "127.0.0.1:80", []string{"missing-port"})
	require.Error(t, err)

	require.Equal(t, IPAny, ClassifyIPVersion("example.com"))
	require.Equal(t, IPV4, ClassifyIPVersion(net.IPv4(127, 0, 0, 1).String()))
}

func TestParseAddressesFlyGlobalServicesForcesIPv4(t *testing.T) {
	addrs, err := ParseAddresses("udp", FlyGlobalServicesHost+":53", nil)
	require.NoError(t, err)
	require.Len(t, addrs, 1)

	addr := addrs[0]
	require.Equal(t, FlyGlobalServicesHost+":53", addr.Address)
	require.Equal(t, IPV4, addr.Version, "fly-global-services must force IPv4 for UDP")
	require.Equal(t, "udp4", addr.Network)
}

func TestParseAddressesRejectsHostname(t *testing.T) {
	_, err := ParseAddresses("tcp", "example.com:80", nil)
	require.Error(t, err)
}
