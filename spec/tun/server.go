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
	ErrHostnameNotFound         = fmt.Errorf("tun: custom hostname is not registered")
)

type Server interface {
	Identity() *protocol.Node
	DialClient(context.Context, *protocol.Link) (net.Conn, error)
	DialInternal(context.Context, *protocol.Node) (net.Conn, error)
}
