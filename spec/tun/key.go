package tun

import (
	"strconv"

	"kon.nect.sh/specter/spec/protocol"
)

func BundleKey(hostname string, num int) string {
	key := "/tunnel/bundle/" + hostname + "/" + strconv.FormatInt(int64(num), 10)
	return key
}

func IdentitiesChordKey(chord *protocol.Node) string {
	key := "/identities/chord/" + chord.GetAddress() + "/" + strconv.FormatUint(chord.GetId(), 10)
	return key
}

func IdentitiesTunKey(tun *protocol.Node) string {
	key := "/identities/tunnel/" + tun.GetAddress() + "/" + strconv.FormatUint(tun.GetId(), 10)
	return key
}
