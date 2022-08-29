package transport

import (
	"context"
	"net"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
)

const (
	ConnectTimeout = time.Second * 10
)

type StreamDelegate struct {
	Identity   *protocol.Node
	Connection net.Conn
}

type DatagramDelegate struct {
	Identity *protocol.Node
	Buffer   []byte
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
}
