package overlay

import (
	"crypto/tls"
	"sync"

	"specter/spec/concurrent"
	"specter/spec/protocol"
	"specter/spec/transport"

	"github.com/lucas-clemente/quic-go"
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

	rpcMap sync.Map
	rpcMu  concurrent.KeyedRWMutex

	qMap sync.Map
	qMu  concurrent.KeyedRWMutex

	rpcChan    chan *transport.Delegate
	directChan chan *transport.Delegate

	server *tls.Config
	client *tls.Config

	closed *atomic.Bool
}
