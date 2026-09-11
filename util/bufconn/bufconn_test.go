package bufconn_test

import (
	"net"
	"sync"
	"testing"
	"time"

	"go.miragespace.co/specter/util/bufconn"
)

func TestClearedDeadlineDoesNotTimeoutLaterIO(t *testing.T) {
	for _, direction := range []string{"read", "write"} {
		t.Run(direction, func(t *testing.T) {
			failures := make(chan error, 32)
			var workers sync.WaitGroup
			for range 32 {
				workers.Go(func() {
					for range 256 {
						if err := exerciseDeadlineReset(direction); err != nil {
							failures <- err
							return
						}
					}
				})
			}
			workers.Wait()
			close(failures)
			for err := range failures {
				t.Errorf("I/O after clearing deadline: %v", err)
			}
		})
	}
}

func exerciseDeadlineReset(direction string) error {
	conn, peer := bufconn.BufferedPipe(1)
	setDeadline := conn.SetReadDeadline
	if direction == "write" {
		_, _ = conn.Write([]byte{0}) // Fill the pipe so the next write waits.
		setDeadline = conn.SetWriteDeadline
	}
	// Expiry can already be running when the deadline is cleared. It must not
	// time out an operation that starts after SetDeadline returns.
	_ = setDeadline(time.Now())
	_ = setDeadline(time.Time{})

	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		time.Sleep(50 * time.Microsecond)
		_, _ = transferByte(peer, direction != "write")
	}()
	_, err := transferByte(conn, direction == "write")
	_ = conn.Close()
	_ = peer.Close()
	<-peerDone
	return err
}

func transferByte(conn net.Conn, write bool) (int, error) {
	data := [1]byte{1}
	if write {
		return conn.Write(data[:])
	}
	return conn.Read(data[:])
}
