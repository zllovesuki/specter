package transport

import "fmt"

var (
	ErrClosed        = fmt.Errorf("transport is already closed")
	ErrNoDirect      = fmt.Errorf("cannot open direct quic connection without address")
	ErrNoCertificate = fmt.Errorf("no verified certificate was provided")
)
