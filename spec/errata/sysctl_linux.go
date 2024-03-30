//go:build linux
// +build linux

package errata

import "os"

func ConfigUDPRecvBuffer() bool {
	// 32MiB
	return os.WriteFile("/proc/sys/net/core/rmem_max", []byte("33554432"), 0o644) == nil
}

func ConfigUDPSendBuffer() bool {
	// 32MiB
	return os.WriteFile("/proc/sys/net/core/wmem_max", []byte("33554432"), 0o644) == nil
}
