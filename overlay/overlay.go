package overlay

import (
	"crypto/tls"

	"github.com/zllovesuki/specter/spec/atomic"
	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/transport"

	"github.com/lucas-clemente/quic-go"
	"github.com/zhangyunhao116/skipmap"
	uberAtomic "go.uber.org/atomic"
	"go.uber.org/zap"
)

type nodeConnection struct {
	peer *protocol.Node
	quic quic.Connection
}

type TransportConfig struct {
	Logger    *zap.Logger
	Endpoint  *protocol.Node
	ServerTLS *tls.Config
	ClientTLS *tls.Config
}

type QUIC struct {
	rpcMap *skipmap.StringMap
	rpcMu  *atomic.KeyedRWMutex

	qMap *skipmap.StringMap
	qMu  *atomic.KeyedRWMutex

	started *uberAtomic.Bool
	closed  *uberAtomic.Bool

	rpcChan    chan *transport.StreamDelegate
	directChan chan *transport.StreamDelegate
	dgramChan  chan *transport.DatagramDelegate

	estChan chan *protocol.Node
	desChan chan *protocol.Node

	TransportConfig
}
