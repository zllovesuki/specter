package tun

import (
	"fmt"

	"kon.nect.sh/specter/spec/protocol"
)

func IdentitiesChordKey(chord *protocol.Node) string {
	return fmt.Sprintf("/identities/chord/%s/%d", chord.GetAddress(), chord.GetId())
}

func IdentitiesTunnelKey(tun *protocol.Node) string {
	return fmt.Sprintf("/identities/tunnel/%s/%d", tun.GetAddress(), tun.GetId())
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
