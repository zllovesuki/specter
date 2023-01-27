package overlay

import (
	"crypto/tls"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util/atomic"

	"github.com/quic-go/quic-go"
	"github.com/zhangyunhao116/skipmap"
	uberAtomic "go.uber.org/atomic"
	"go.uber.org/zap"
)

type nodeConnection struct {
	peer *protocol.Node
	quic quic.EarlyConnection
}

type TransportConfig struct {
	Logger    *zap.Logger
	Endpoint  *protocol.Node
	ServerTLS *tls.Config
	ClientTLS *tls.Config
}

type QUIC struct {
	cachedConnections *skipmap.StringMap[*nodeConnection]
	cachedMutex       *atomic.KeyedRWMutex

	started *uberAtomic.Bool
	closed  *uberAtomic.Bool

	streamChan chan *transport.StreamDelegate
	dgramChan  chan *transport.DatagramDelegate

	TransportConfig
}
