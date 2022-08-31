//go:build !android
// +build !android

package errata

func ConfigDNS() bool {
	return false
}
