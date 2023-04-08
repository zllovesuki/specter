//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"

	"github.com/stretchr/testify/mock"
)

type MemoryTransport struct {
	Identify    *protocol.Node
	Other       chan *transport.StreamDelegate
	Self        chan *transport.StreamDelegate
	Certificate *x509.Certificate
	mock.Mock
}

var _ transport.Transport = (*MemoryTransport)(nil)
var _ transport.ClientTransport = (*MemoryTransport)(nil)

// SelfTransport returns a transport.Transport that when .DialStream() is invoked, .AcceptStream()
// on the same Transport will receive the net.Conn
func SelfTransport() *MemoryTransport {
	s := make(chan *transport.StreamDelegate, 1)
	t := &MemoryTransport{
		Other: s,
		Self:  s,
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
		Self:  s1,
	}
	t2 := &MemoryTransport{
		Other: s1,
		Self:  s2,
	}
	return t1, t2
}

func (t *MemoryTransport) WithCertificate(cert *x509.Certificate) {
	t.Certificate = cert
}

func (t *MemoryTransport) WithClientCertificate(cert tls.Certificate) error {
	args := t.Called(cert)
	return args.Error(0)
}

func (t *MemoryTransport) Identity() *protocol.Node {
	return t.Identify
}

func (t *MemoryTransport) DialStream(ctx context.Context, peer *protocol.Node, kind protocol.Stream_Type) (net.Conn, error) {
	c1, c2 := net.Pipe()
	select {
	case t.Other <- &transport.StreamDelegate{
		Conn:        c1,
		Identity:    peer,
		Kind:        kind,
		Certificate: t.Certificate,
	}:
	default:
		panic(fmt.Sprintf("blocked on dialing %s", peer.String()))
	}
	return c2, nil
}

func (t *MemoryTransport) AcceptStream() <-chan *transport.StreamDelegate {
	return t.Self
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
