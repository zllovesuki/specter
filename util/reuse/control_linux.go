//go:build linux

package reuse

import (
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func Control(network, address string, c syscall.RawConn) error {
	if !strings.HasPrefix(network, "tcp") && !strings.HasPrefix(network, "udp") {
		return nil
	}

	var err error
	c.Control(func(fd uintptr) {
		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
		if err != nil {
			return
		}

		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		if err != nil {
			return
		}
	})
	return err
}
