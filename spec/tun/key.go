package tun

import "strconv"

func Key(hostname string, num int) string {
	key := strconv.FormatInt(int64(num), 10) + "/" + hostname + "/tun/"
	return key
}
