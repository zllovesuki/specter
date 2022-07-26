package overlay

import (
	"crypto/tls"

	"kon.nect.sh/specter/spec/atomic"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"

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
	rpcMap *skipmap.StringMap[rpc.RPC]
	rpcMu  *atomic.KeyedRWMutex

	qMap *skipmap.StringMap[*nodeConnection]
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
