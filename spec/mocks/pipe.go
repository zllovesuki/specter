//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"context"
	"net"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
)

type MemoryTransport struct {
	Identify *protocol.Node
	Other    chan *transport.StreamDelegate
}

// SelfTransport returns a transport.Transport that when .DialStream() is invoked, .AcceptStream()
// on the same Transport will receive the net.Conn
func SelfTransport() *MemoryTransport {
	s := make(chan *transport.StreamDelegate, 1)
	t := &MemoryTransport{
		Other: s,
	}
	return t
}

// PipeTransport returns two transport.Transport that when one side's .DialStream() is invoked,
// .AcceptStream() on the other Transport will receive the net.Conn
func PipeTransport() (*MemoryTransport, *MemoryTransport) {
	s1 := make(chan *transport.StreamDelegate, 1)
	s2 := make(chan *transport.StreamDelegate, 1)
	t1 := &MemoryTransport{
		Other: s2,
	}
	t2 := &MemoryTransport{
		Other: s1,
	}
	return t1, t2
}

func (t *MemoryTransport) Identity() *protocol.Node {
	return t.Identify
}

func (t *MemoryTransport) DialStream(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
	c1, c2 := net.Pipe()
	t.Other <- &transport.StreamDelegate{
		Connection: c1,
		Identity:   peer,
		Kind:       kind,
	}
	return c2, nil
}

func (t *MemoryTransport) AcceptStream() <-chan *transport.StreamDelegate {
	return t.Other
}

func (t *MemoryTransport) SupportDatagram() bool {
	panic("not implemented") // TODO: Implement
}

func (t *MemoryTransport) ReceiveDatagram() <-chan *transport.DatagramDelegate {
	panic("not implemented") // TODO: Implement
}

func (t *MemoryTransport) SendDatagram(_ *protocol.Node, _ []byte) error {
	panic("not implemented") // TODO: Implement
}
