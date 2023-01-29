//go:build !linux
// +build !linux

package errata

func ConfigUDPBuffer() bool {
	return false
}
