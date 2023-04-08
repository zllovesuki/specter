package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"time"

	"kon.nect.sh/specter/spec/protocol"
)

const (
	ConnectTimeout     = time.Second * 10
	RTTMeasureInterval = time.Second * 3
)

type StreamDelegate struct {
	net.Conn                         // The backing bytes stream of the delegation
	Certificate *x509.Certificate    // The verified peer certificate, if any
	Identity    *protocol.Node       // The identity that the peer claims to be. Implementation may use mTLS to verify
	Kind        protocol.Stream_Type // Type of the bytes stream of the delegation
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

type ClientTransport interface {
	Transport
	WithClientCertificate(cert tls.Certificate) error
}
