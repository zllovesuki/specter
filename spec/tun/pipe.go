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

func Pipe(src, dst io.ReadWriteCloser) <-chan error {
	err := make(chan error, 2)
	wg := &sync.WaitGroup{}
	wg.Add(2)

	go pipe(wg, err, src, dst)
	go pipe(wg, err, dst, src)
	go func() {
		wg.Wait()
		close(err)
	}()

	return err
}

func pipe(wg *sync.WaitGroup, errChan chan<- error, dst, src io.ReadWriteCloser) {
	defer wg.Done()

	buf := pool.Get(BufferSize)
	defer pool.Put(buf)

	_, err := io.CopyBuffer(src, dst, buf)

	src.Close()
	dst.Close()

	if err != nil {
		errChan <- err
	}
}
