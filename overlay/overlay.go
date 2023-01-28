package overlay

import (
	"crypto/tls"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util/atomic"

	"github.com/puzpuzpuz/xsync/v2"
	"github.com/quic-go/quic-go"
	uberAtomic "go.uber.org/atomic"
	"go.uber.org/zap"
)

type nodeConnection struct {
	peer      *protocol.Node
	quic      quic.EarlyConnection
	direction direction
}

type TransportConfig struct {
	Logger    *zap.Logger
	Endpoint  *protocol.Node
	ServerTLS *tls.Config
	ClientTLS *tls.Config
}

type QUIC struct {
	cachedConnections *xsync.MapOf[string, *nodeConnection]
	cachedMutex       *atomic.KeyedRWMutex

	started *uberAtomic.Bool
	closed  *uberAtomic.Bool

	streamChan chan *transport.StreamDelegate
	dgramChan  chan *transport.DatagramDelegate

	TransportConfig
}
