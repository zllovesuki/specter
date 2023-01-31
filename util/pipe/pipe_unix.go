//go:build unix

package pipe

import (
	"context"
	"net"
)

func DialPipe(ctx context.Context, path string) (net.Conn, error) {
	dialer := &net.Dialer{}
	return dialer.DialContext(ctx, "unix", path)
}

func ListenPipe(path string) (net.Listener, error) {
	return net.Listen("unix", path)
}
