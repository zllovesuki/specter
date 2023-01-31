//go:build !(windows || unix)

package pipe

import "errors"

func DialPipe(ctx context.Context, path string) (net.Conn, error) {
	return nil, errors.New("Not implemented")
}

func ListenPipe(path string) (net.Listener, error) {
	return nil, errors.New("Not implemented")
}
