package overlay

import (
	"context"
	"crypto/tls"
	"net"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/util/atomic"

	"github.com/quic-go/quic-go"
	"github.com/zhangyunhao116/skipmap"
	uberAtomic "go.uber.org/atomic"
	"go.uber.org/zap"
)

type nodeConnection struct {
	peer      *protocol.Node
	quic      *quic.Conn
	direction direction
	version   string
}

type QuicDialer interface {
	DialEarly(ctx context.Context, addr net.Addr, tlsConf *tls.Config, config *quic.Config) (*quic.Conn, error)
}

type TransportConfig struct {
	Logger                 *zap.Logger
	QuicTransport          QuicDialer
	Endpoint               *protocol.Node
	ServerTLS              *tls.Config
	ClientTLS              *tls.Config
	RTTRecorder            rtt.Recorder
	VirtualTransport       bool
	UseCertificateIdentity bool
}

type QUIC struct {
	cachedConnections *skipmap.StringMap[*nodeConnection]
	cachedMutex       *atomic.KeyedRWMutex

	started *uberAtomic.Bool
	closed  *uberAtomic.Bool

	streamChan chan *transport.StreamDelegate
	dgramChan  chan *transport.DatagramDelegate

	rttChan chan *transport.DatagramDelegate
	rttMap  *skipmap.StringMap[*skipmap.Uint64Map[int64]]

	clientCert uberAtomic.Value // tls.Certificate

	TransportConfig
}
