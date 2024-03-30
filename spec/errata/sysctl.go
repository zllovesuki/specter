//go:build !linux
// +build !linux

package errata

func ConfigUDPRecvBuffer() bool {
	return false
}

func ConfigUDPSendBuffer() bool {
	return false
}
