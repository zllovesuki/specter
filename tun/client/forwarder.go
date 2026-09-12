package client

import (
	"context"
	"net"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/tun"

	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type forwarder struct {
	logger     *zap.Logger
	rootDomain *atomic.String
	proxies    *skipmap.StringMap[*httpProxy]
}

func newForwarder(logger *zap.Logger) *forwarder {
	return &forwarder{
		logger:     logger,
		rootDomain: atomic.NewString(""),
		proxies:    skipmap.NewString[*httpProxy](),
	}
}

func receiveLink(logger *zap.Logger, delegation net.Conn) (*protocol.Link, bool) {
	link := &protocol.Link{}
	if err := rpc.BoundedReceive(delegation, link, 1024); err != nil {
		logger.Error("Receiving link information from gateway", zap.Error(err))
		delegation.Close()
		return nil, false
	}
	return link, true
}

func (f *forwarder) handleLink(ctx context.Context, link *protocol.Link, delegation net.Conn, u route) error {
	hostname := link.GetHostname()
	f.logger.Info("Incoming connection from gateway",
		zap.String("protocol", link.GetAlpn().String()),
		zap.String("hostname", link.GetHostname()),
		zap.String("remote", link.GetRemote()))

	switch link.GetAlpn() {
	case protocol.Link_HTTP:
		f.getHTTPProxy(ctx, hostname, u).acceptor.Handle(delegation)

	case protocol.Link_TCP:
		f.forwardStream(ctx, hostname, delegation, u)

	default:
		f.logger.Error("Unknown alpn for forwarding", zap.String("alpn", link.GetAlpn().String()))
		delegation.Close()
		return tun.ErrDestinationNotFound
	}

	return nil
}

func (f *forwarder) closeAll() {
	f.proxies.Range(func(key string, proxy *httpProxy) bool {
		f.logger.Info("Shutting down proxy", zap.String("hostname", key))
		proxy.acceptor.Close()
		return true
	})
}
