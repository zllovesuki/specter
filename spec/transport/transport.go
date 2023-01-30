package transport

import (
	"context"
	"net"
	"time"

	"kon.nect.sh/specter/spec/protocol"
)

const (
	ConnectTimeout     = time.Second * 10
	RTTMeasureInterval = time.Second * 3
)

type StreamDelegate struct {
	net.Conn
	Identity *protocol.Node
	Kind     protocol.Stream_Type
}

type DatagramDelegate struct {
	Identity *protocol.Node
	Buffer   []byte
}

type Transport interface {
	Identity() *protocol.Node

	DialStream(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error)
	AcceptStream() <-chan *StreamDelegate

	SupportDatagram() bool
	ReceiveDatagram() <-chan *DatagramDelegate
	SendDatagram(*protocol.Node, []byte) error
}
