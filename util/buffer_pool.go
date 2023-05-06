package util

import (
	"net/http/httputil"

	pool "github.com/libp2p/go-buffer-pool"
)

type BufferPool struct {
	bufferSize int
}

var _ httputil.BufferPool = (*BufferPool)(nil)

func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		bufferSize: size,
	}
}

func (b *BufferPool) Get() []byte {
	return pool.Get(b.bufferSize)
}

func (b *BufferPool) Put(buf []byte) {
	pool.Put(buf)
}
