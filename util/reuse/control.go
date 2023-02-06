//go:build !(windows || linux)

package reuse

import (
	"syscall"
)

func Control(network, address string, c syscall.RawConn) error {
	return nil
}
