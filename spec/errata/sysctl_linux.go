//go:build linux
// +build linux

package errata

import "os"

func ConfigUDPBuffer() bool {
	return os.WriteFile("/proc/sys/net/core/rmem_max", []byte("2500000"), 0o644) == nil
}
