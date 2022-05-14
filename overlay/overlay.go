package overlay

import (
	"crypto/tls"
	"net"
	"sync"

	"specter/rpc"
	"specter/spec/protocol"

	"github.com/lucas-clemente/quic-go"
	"go.uber.org/zap"
)

type nodeConnection struct {
	peer *protocol.Node
	quic quic.Connection
}

type RPCHandshakeFunc func(*rpc.RPC) error

type Stream struct {
	Connection quic.Stream
	Remote     net.Addr
}

type Transport struct {
	logger *zap.Logger

	qMap map[string]*nodeConnection
	qMu  sync.RWMutex

	rpcMap map[string]*rpc.RPC
	rpcMu  sync.RWMutex

	peerRpcChan   chan Stream
	clientRpcChan chan Stream
	tunnelChan    chan Stream

	server *tls.Config
	client *tls.Config
}
