package tun

import (
	"strconv"

	"specter/spec/protocol"
)

func BundleKey(hostname string, num int) string {
	key := strconv.FormatInt(int64(num), 10) + "/" + hostname + "/bundle/tun/"
	return key
}

func IdentitiesChordKey(chord *protocol.Node) string {
	key := strconv.FormatUint(chord.GetId(), 10) + "/" + chord.GetAddress() + "/identities/chord/"
	return key
}

func IdentitiesTunKey(tun *protocol.Node) string {
	key := strconv.FormatUint(tun.GetId(), 10) + "/" + tun.GetAddress() + "/identities/tun/"
	return key
}
