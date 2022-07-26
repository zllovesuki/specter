package transport

import (
	"context"
	"net"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
)

type StreamDelegate struct {
	Connection net.Conn
	Identity   *protocol.Node
}

type DatagramDelegate struct {
	Buffer   []byte
	Identity *protocol.Node
}

type Transport interface {
	Identity() *protocol.Node

	DialRPC(ctx context.Context, peer *protocol.Node, hs rpc.RPCHandshakeFunc) (rpc.RPC, error)
	DialDirect(ctx context.Context, peer *protocol.Node) (net.Conn, error)

	RPC() <-chan *StreamDelegate
	Direct() <-chan *StreamDelegate

	SupportDatagram() bool
	ReceiveDatagram() <-chan *DatagramDelegate
	SendDatagram(*protocol.Node, []byte) error

	TransportEstablished() <-chan *protocol.Node
	TransportDestroyed() <-chan *protocol.Node

	Accept(ctx context.Context) error

	Stop()
}
