package overlay

import (
	"crypto/tls"
	"sync"

	"specter/spec/protocol"
	"specter/spec/rpc"
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

	rpcMap map[string]rpc.RPC
	rpcMu  sync.RWMutex

	rpcChan    chan *transport.Delegate
	directChan chan *transport.Delegate

	reuseMap sync.Map

	server *tls.Config
	client *tls.Config

	closed *atomic.Bool
}
