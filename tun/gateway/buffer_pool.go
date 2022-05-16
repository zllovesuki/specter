package gateway

import (
	"net/http/httputil"

	pool "github.com/libp2p/go-buffer-pool"
)

const (
	bufferSize = 8 * 1024
)

type bufferPool struct{}

var _ httputil.BufferPool = (*bufferPool)(nil)

func newBufferPool() *bufferPool {
	return &bufferPool{}
}

func (b *bufferPool) Get() []byte {
	return pool.Get(bufferSize)
}

func (b *bufferPool) Put(buf []byte) {
	pool.Put(buf)
}
