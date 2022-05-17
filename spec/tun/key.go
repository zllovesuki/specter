package tun

import (
	"strconv"

	"github.com/zllovesuki/specter/spec/protocol"
)

func BundleKey(hostname string, num int) string {
	key := strconv.FormatInt(int64(num), 10) + "/" + hostname + "/tunnel/bundle/"
	return key
}

func IdentitiesChordKey(chord *protocol.Node) string {
	key := strconv.FormatUint(chord.GetId(), 10) + "/" + chord.GetAddress() + "/chord/identities/"
	return key
}

func IdentitiesTunKey(tun *protocol.Node) string {
	key := strconv.FormatUint(tun.GetId(), 10) + "/" + tun.GetAddress() + "/tun/identities/"
	return key
}
