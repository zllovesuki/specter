package tun

import (
	"context"
	"fmt"
	"net"

	"kon.nect.sh/specter/spec/protocol"
)

var (
	ErrDestinationNotFound      = fmt.Errorf("tun: tunnel not found on specter network")
	ErrTunnelClientNotConnected = fmt.Errorf("tun: tunnel client is not connected to any nodes")
)

type Server interface {
	Dial(context.Context, *protocol.Link) (net.Conn, error)
	Accept(context.Context)
	Stop()
}
