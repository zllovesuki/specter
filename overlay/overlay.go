package overlay

import (
	"crypto/tls"

	"specter/spec/concurrent"
	"specter/spec/protocol"
	"specter/spec/transport"

	"github.com/lucas-clemente/quic-go"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type nodeConnection struct {
	peer *protocol.Node
	quic quic.Connection
}

type QUIC struct {
	logger *zap.Logger

	self *protocol.Node

	rpcMap *skipmap.StringMap
	rpcMu  *concurrent.KeyedRWMutex

	qMap *skipmap.StringMap
	qMu  *concurrent.KeyedRWMutex

	rpcChan    chan *transport.StreamDelegate
	directChan chan *transport.StreamDelegate
	dgramChan  chan *transport.DatagramDelegate

	server *tls.Config
	client *tls.Config

	closed *atomic.Bool
}
