package tun

import (
	"fmt"

	"kon.nect.sh/specter/spec/protocol"
)

func DestinationByChordKey(chord *protocol.Node) string {
	return fmt.Sprintf("/destination/chord/%s", chord.GetAddress())
}

func DestinationByTunnelKey(tunnel *protocol.Node) string {
	return fmt.Sprintf("/destination/tunnel/%s", tunnel.GetAddress())
}

func RoutingKey(hostname string, num int) string {
	return fmt.Sprintf("/tunnel/bundle/%s/%d", hostname, num)
}

func ClientTokenKey(token *protocol.ClientToken) string {
	return fmt.Sprintf("/tunnel/client/token/%s", token.GetToken())
}

func ClientHostnamesPrefix(token *protocol.ClientToken) string {
	return fmt.Sprintf("/tunnel/client/hostnames/%s", token.GetToken())
}

func ClientLeaseKey(token *protocol.ClientToken) string {
	return fmt.Sprintf("/tunnel/client/lease/%s", token.GetToken())
}
