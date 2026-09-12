//go:build !no_mocks

package mocks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"sync"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/util/bufconn"

	"github.com/stretchr/testify/mock"
)

type MemoryTransport struct {
	Identify    *protocol.Node
	Other       chan *transport.StreamDelegate
	Self        chan *transport.StreamDelegate
	Physical    transport.PhysicalConn
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
	c1, c2 := bufconn.BufferedPipe(8192)
	if t.Physical != nil {
		c1 = &physicalPipe{c1, t.Physical}
		c2 = &physicalPipe{c2, t.Physical}
	}
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

func (t *MemoryTransport) ListConnected() []transport.ConnectedPeer {
	panic("not implemented") // TODO: Implement
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

// PhysicalConn is a controllable connection lifetime for session tests.
type PhysicalConn struct {
	deliver func(*transport.StreamDelegate)
	done    chan struct{}
	mu      sync.Mutex
	err     error
	streams []net.Conn
}

func NewPhysicalConn(deliver func(*transport.StreamDelegate)) *PhysicalConn {
	return &PhysicalConn{
		deliver: deliver,
		done:    make(chan struct{}),
	}
}

func (p *PhysicalConn) Done() <-chan struct{} { return p.done }

func (p *PhysicalConn) Err() error { p.mu.Lock(); defer p.mu.Unlock(); return p.err }

func (p *PhysicalConn) Close(reason string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return nil
	}
	p.err = errors.New(reason)
	close(p.done)
	for _, c := range p.streams {
		c.Close()
	}
	p.streams = nil
	return nil
}

func (p *PhysicalConn) OpenStream(kind protocol.Stream_Type) (net.Conn, error) {
	p.mu.Lock()
	if p.err != nil {
		p.mu.Unlock()
		return nil, transport.ErrClosed
	}
	c1, c2 := bufconn.BufferedPipe(8192)
	p.streams = append(p.streams, c1, c2)
	p.mu.Unlock()
	p.deliver(&transport.StreamDelegate{
		Conn: &physicalPipe{c1, p},
		Kind: kind,
	})
	return &physicalPipe{c2, p}, nil
}

type physicalPipe struct {
	net.Conn
	pc transport.PhysicalConn
}

func (p *physicalPipe) PhysicalConn() transport.PhysicalConn { return p.pc }
