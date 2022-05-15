package tun

import (
	"io"
	"sync"

	pool "github.com/libp2p/go-buffer-pool"
)

const (
	BufferSize = 4096
)

func Pipe(src, dst io.ReadWriteCloser) {
	wg := &sync.WaitGroup{}
	wg.Add(2)

	go pipe(wg, src, dst)
	go pipe(wg, dst, src)

	wg.Wait()
}

func pipe(wg *sync.WaitGroup, dst, src io.ReadWriteCloser) {
	defer wg.Done()

	buf := pool.Get(BufferSize)
	defer pool.Put(buf)

	io.CopyBuffer(src, dst, buf)

	src.Close()
	dst.Close()
}
