package tun

import (
	"context"
	"fmt"
	"net"

	"kon.nect.sh/specter/spec/protocol"
)

var (
	ErrDestinationNotFound = fmt.Errorf("tunnel not found on chord")
)

type Server interface {
	Dial(context.Context, *protocol.Link) (net.Conn, error)
	Accept(context.Context)
	Stop()
}
