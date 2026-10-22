package tun

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	pool "github.com/libp2p/go-buffer-pool"
)

const (
	BufferSize = 1024 * 16
)

func IsTimeout(err error) bool {
	t := errors.Is(err, context.DeadlineExceeded)
	if t {
		return t
	}
	if e, ok := err.(net.Error); ok {
		return e.Timeout()
	}
	return false
}

func Pipe(a, b io.ReadWriteCloser) <-chan error {
	errChan := make(chan error, 2)
	var wg sync.WaitGroup
	var closeOnce sync.Once

	copyOneWay := func(src, dst io.ReadWriteCloser) {
		defer wg.Done()

		buf := pool.Get(BufferSize)
		defer pool.Put(buf)

		_, err := io.CopyBuffer(dst, src, buf)

		// The first completion owns teardown. In particular, the reverse copy's
		// cancellation must not abort a successfully completed destination write.
		closeOnce.Do(func() {
			if conn, ok := dst.(interface{ CloseGracefully() error }); ok && err == nil {
				conn.CloseGracefully()
			} else {
				dst.Close()
			}
			src.Close()
		})

		if err != nil {
			errChan <- err
		}
	}

	wg.Add(2)
	go copyOneWay(a, b)
	go copyOneWay(b, a)
	go func() {
		wg.Wait()
		close(errChan)
	}()

	return errChan
}
