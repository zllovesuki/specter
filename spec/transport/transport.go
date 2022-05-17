package transport

import (
	"context"
	"net"

	"specter/spec/protocol"
	"specter/spec/rpc"
)

type StreamDelegate struct {
	Connection net.Conn
	Identity   *protocol.Node
}

type DatagramDelegate struct {
	Buffer   []byte
	Identity *protocol.Node
}

type EventDelegate interface {
	Created(*protocol.Node)
	Removed(*protocol.Node)
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

	Accept(ctx context.Context) error

	Stop()
}
