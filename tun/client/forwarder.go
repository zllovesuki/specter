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
	routes     *skipmap.StringMap[*route]
}

type route struct {
	Tunnel
	proxy *httpProxy
}

func newForwarder(logger *zap.Logger) *forwarder {
	return &forwarder{
		logger:     logger,
		rootDomain: atomic.NewString(""),
		routes:     skipmap.NewString[*route](),
	}
}

func (f *forwarder) newRoute(hostname string, tunnel Tunnel) *route {
	r := &route{Tunnel: tunnel, proxy: f.newHTTPProxy(hostname, tunnel)}
	go r.proxy.forwarder.Serve(r.proxy.acceptor)
	return r
}

// setRoutes publishes complete entries. Callers serialize configuration updates;
// readers only need the skipmap lookup and never create a proxy.
func (f *forwarder) setRoutes(tunnels []Tunnel) {
	next := make(map[string]Tunnel, len(tunnels))
	for _, tunnel := range tunnels {
		next[tunnel.Hostname] = tunnel
	}
	for hostname, tunnel := range next {
		old, loaded := f.routes.Load(hostname)
		if loaded {
			previous := old.Tunnel
			// Validation reparses URLs; compare the configured fields instead.
			previous.parsed = tunnel.parsed
			if previous == tunnel {
				continue
			}
		}
		f.routes.Store(hostname, f.newRoute(hostname, tunnel))
		if loaded {
			old.proxy.close()
		}
	}
	f.routes.Range(func(hostname string, old *route) bool {
		if _, ok := next[hostname]; !ok {
			f.routes.Delete(hostname)
			old.proxy.close()
		}
		return true
	})
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

func (f *forwarder) handleIncomingDelegation(ctx context.Context, link *protocol.Link, delegation net.Conn) error {
	hostname := link.GetHostname()
	r, ok := f.routes.Load(hostname)
	if !ok {
		f.logger.Error("Unknown hostname in connection", zap.String("hostname", hostname))
		delegation.Close()
		return tun.ErrDestinationNotFound
	}
	return f.handleLink(ctx, link, delegation, r)
}

func (f *forwarder) handleLink(ctx context.Context, link *protocol.Link, delegation net.Conn, r *route) error {
	f.logger.Info("Incoming connection from gateway",
		zap.String("protocol", link.GetAlpn().String()),
		zap.String("hostname", link.GetHostname()),
		zap.String("remote", link.GetRemote()))

	switch link.GetAlpn() {
	case protocol.Link_HTTP:
		r.proxy.acceptor.Handle(delegation)
	case protocol.Link_TCP:
		f.forwardStream(ctx, link.GetHostname(), delegation, r.Tunnel)
	default:
		f.logger.Error("Unknown alpn for forwarding", zap.String("alpn", link.GetAlpn().String()))
		delegation.Close()
		return tun.ErrDestinationNotFound
	}
	return nil
}

func (f *forwarder) closeAll() {
	f.routes.Range(func(hostname string, r *route) bool {
		f.routes.Delete(hostname)
		r.proxy.close()
		return true
	})
}
