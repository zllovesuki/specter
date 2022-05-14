package overlay

import (
	"crypto/tls"
	"sync"

	"specter/spec"
	"specter/spec/protocol"
	"specter/spec/transport"

	"github.com/lucas-clemente/quic-go"
	"go.uber.org/zap"
)

type nodeConnection struct {
	peer *protocol.Node
	quic quic.Connection
}

type QUIC struct {
	logger *zap.Logger

	qMap map[string]*nodeConnection
	qMu  sync.RWMutex

	rpcMap map[string]spec.RPC
	rpcMu  sync.RWMutex

	rpcChan    chan transport.Stream
	directChan chan transport.Stream

	server *tls.Config
	client *tls.Config
}
