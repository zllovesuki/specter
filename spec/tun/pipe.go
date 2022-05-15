package tun

import (
	"io"
	"net"
	"sync"
)

func Pipe(src, dst net.Conn) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(src, dst)
	}()
	go func() {
		defer wg.Done()
		io.Copy(dst, src)
	}()

	wg.Wait()
}
