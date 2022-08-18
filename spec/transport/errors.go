package transport

import "fmt"

var (
	ErrClosed   = fmt.Errorf("transport is already closed")
	ErrNoDirect = fmt.Errorf("cannot open direct quic connection without address")
)
