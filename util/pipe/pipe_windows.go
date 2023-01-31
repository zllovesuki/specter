//go:build windows

package pipe

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func DialPipe(ctx context.Context, path string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, path)
}

func ListenPipe(path string) (net.Listener, error) {
	return winio.ListenPipe(path, &winio.PipeConfig{
		InputBufferSize:  8 * 1024,
		OutputBufferSize: 8 * 1024,
	})
}
