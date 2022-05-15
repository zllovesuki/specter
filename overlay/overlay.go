package overlay

import (
	"crypto/tls"
	"net"
	"sync"

	"specter/spec/protocol"
	"specter/spec/rpc"

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

	qMap map[string]*nodeConnection
	qMu  sync.RWMutex

	rpcMap map[string]rpc.RPC
	rpcMu  sync.RWMutex

	rpcChan    chan net.Conn
	directChan chan net.Conn

	server *tls.Config
	client *tls.Config

	closed *atomic.Bool
}
