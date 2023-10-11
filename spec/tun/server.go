package tun

import (
	"context"
	"fmt"
	"net"

	"go.miragespace.co/specter/spec/protocol"
)

var (
	ErrLookupFailed             = fmt.Errorf("tun: failed to lookup tunnel on specter network")
	ErrDestinationNotFound      = fmt.Errorf("tun: tunnel not found on specter network")
	ErrTunnelClientNotConnected = fmt.Errorf("tun: tunnel client is not connected to any nodes")
	ErrHostnameNotFound         = fmt.Errorf("tun: custom hostname is not registered")
)

type Server interface {
	Identity() *protocol.Node
	DialClient(context.Context, *protocol.Link) (net.Conn, error)
	DialInternal(context.Context, *protocol.Node) (net.Conn, error)
}
