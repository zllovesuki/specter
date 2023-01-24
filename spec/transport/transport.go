package transport

import (
	"context"
	"net"
	"time"

	"kon.nect.sh/specter/spec/protocol"
)

const (
	ConnectTimeout = time.Second * 10
)

type StreamDelegate struct {
	Identity   *protocol.Node
	Connection net.Conn
	Kind       protocol.Stream_Type
}

type DatagramDelegate struct {
	Identity *protocol.Node
	Buffer   []byte
}

type Transport interface {
	Identity() *protocol.Node

	Dial(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error)
	AcceptStream() <-chan *StreamDelegate

	SupportDatagram() bool
	ReceiveDatagram() <-chan *DatagramDelegate
	SendDatagram(*protocol.Node, []byte) error
}
