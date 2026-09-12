package server

import (
	"context"
	"errors"
	"net"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/cipher"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/util/acceptor"

	"github.com/Yiling-J/theine-go"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// Gateway procedure:
// use the hostname to lookup chordHash(hostname-[1..3])
// lookup those keys sequentially on DHT
// if all failed, that means hostname does not point to a valid tunnel
// if any of the one is good, fetch (ClientIdentity, ServerIdentity) from DHT
// if ServerIdentity is ourself, we have a direct connection to client
// otherwise, DialDirect to that server to get a tunnel Stream
// in either case, pipe the gateway connection to (direct|indirect) Stream
// tunnel is now established

type Config struct {
	Logger          *zap.Logger
	ParentContext   context.Context
	Chord           chord.VNode
	TunnelTransport transport.Transport
	ChordTransport  transport.Transport
	Resolver        tun.DNSResolver
	CertProvider    cipher.CertProvider
	Apex            string
	Acme            string
}

type Server struct {
	sessions       *sessionRegistry
	ephemeralLoads chan struct{}
	rpcAcceptor    *acceptor.HTTP2Acceptor
	routeCache     *theine.Cache[string, *routesResult]
	routeLoads     singleflight.Group
	keylessCache   *theine.LoadingCache[string, keylessCertResult]
	Config
}

var _ tun.Server = (*Server)(nil)

var _ protocol.TunnelService = (*Server)(nil)

func New(cfg Config) *Server {
	s := &Server{
		Config:         cfg,
		sessions:       newSessionRegistry(),
		ephemeralLoads: make(chan struct{}, 16),
		rpcAcceptor:    acceptor.NewH2Acceptor(nil),
	}
	s.initRouteCache()
	s.initKeylessCache()
	return s
}

func (s *Server) Identity() *protocol.Node {
	return s.TunnelTransport.Identity()
}

func (s *Server) AttachRouter(ctx context.Context, router *transport.StreamRouter) {
	router.HandleChord(protocol.Stream_PROXY, nil, func(delegate *transport.StreamDelegate) {
		l := s.Logger.With(
			zap.Object("peer", delegate.Identity),
			zap.String("remote", delegate.RemoteAddr().String()),
			zap.String("local", delegate.LocalAddr().String()),
		)
		defer func() {
			if err := recover(); err != nil {
				l.Warn("Panic recovered while handling proxy connection", zap.Any("error", err))
			}
		}()
		s.handleProxyConn(ctx, delegate)
	})
	router.HandleTunnel(protocol.Stream_DIRECT, func(delegate *transport.StreamDelegate) {
		// client uses this to register connection
		// but it is a no-op on the server side
		delegate.Close()
	})
	s.attachRPC(ctx, router)
}

func (s *Server) MustRegister(ctx context.Context) {
	s.Logger.Info("Publishing destinations to chord")

	publishCtx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	if err := s.publishDestinations(publishCtx); err != nil {
		s.Logger.Panic("Error publishing destinations", zap.Error(err))
	}

	s.Logger.Info("specter server started")
}

func (s *Server) Stop() {
	defer s.routeCache.Close()
	defer s.keylessCache.Close()

	s.rpcAcceptor.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	s.unpublishDestinations(ctx)
}

func (s *Server) handleProxyConn(ctx context.Context, delegation *transport.StreamDelegate) {
	var err error
	var clientConn net.Conn

	defer func() {
		tun.SendStatusProto(delegation, err)
		if err != nil {
			delegation.Close()
			return
		}
		tun.Pipe(delegation, clientConn)
	}()

	route := &protocol.TunnelRoute{}
	err = rpc.BoundedReceive(delegation, route, 2048)
	if err != nil {
		s.Logger.Error("Error receiving remote tunnel negotiation", zap.Error(err))
		return
	}

	l := s.Logger.With(
		zap.String("hostname", route.GetHostname()),
		zap.Uint64("client", route.GetClientDestination().GetId()),
	)
	l.Debug("Received proxy stream from remote node",
		zap.Object("remote_chord", delegation.Identity),
		zap.Object("chord", route.GetChordDestination()),
		zap.Object("tunnel", route.GetTunnelDestination()))

	if route.GetTunnelDestination().GetAddress() != s.TunnelTransport.Identity().GetAddress() {
		l.Warn("Received remote connection for the wrong server",
			zap.String("expected", s.TunnelTransport.Identity().GetAddress()),
			zap.String("got", route.GetTunnelDestination().GetAddress()),
		)
		err = tun.ErrDestinationNotFound
		return
	}

	clientConn, err = s.dialDirect(ctx, route)
	if err != nil && !tun.IsNoDirect(err) {
		l.Error("Error dialing connection to connected client", zap.Error(err))
	}
}

func (s *Server) getConn(ctx context.Context, route *protocol.TunnelRoute, link *protocol.Link) (net.Conn, error) {
	l := s.Logger.With(
		zap.String("hostname", route.GetHostname()),
		zap.Uint64("client", route.GetClientDestination().GetId()),
	)

	var conn net.Conn
	var err error
	direct := route.GetTunnelDestination().GetAddress() == s.TunnelTransport.Identity().GetAddress()
	if direct {
		l.Debug("client is connected to us, opening direct stream")
		conn, err = s.dialDirect(ctx, route)
	} else {
		l.Debug("client is connected to remote node, opening proxy stream",
			zap.Object("chord", route.GetChordDestination()),
			zap.Object("tunnel", route.GetTunnelDestination()))

		conn, err = s.ChordTransport.DialStream(ctx, route.GetChordDestination(), protocol.Stream_PROXY)
	}
	if err != nil {
		return nil, err
	}

	// Transports may retain the dialing context for a physical connection. Keep
	// this stream's negotiation timeout separate from that connection lifecycle.
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	// Bound stream negotiation, including writes, and close failed streams before
	// trying another route. Cancelled callers also interrupt blocked I/O.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	ready := false
	defer func() {
		if !ready {
			conn.Close()
		}
	}()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if !direct {
		if err := rpc.Send(conn, route); err != nil {
			l.Error("sending remote tunnel negotiation", zap.Error(err))
			return nil, err
		}
		status := &protocol.TunnelStatus{}
		if err := rpc.BoundedReceive(conn, status, 1024); err != nil {
			l.Error("Error receiving remote tunnel status", zap.Error(err))
			return nil, err
		}
		switch status.GetStatus() {
		case protocol.TunnelStatusCode_STATUS_OK:
		case protocol.TunnelStatusCode_NO_DIRECT:
			return nil, tun.ErrTunnelClientNotConnected
		default:
			return nil, errors.New(status.GetError())
		}
	}
	if err := rpc.Send(conn, link); err != nil {
		return nil, err
	}
	if !stop() {
		return nil, ctx.Err()
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	ready = true
	return conn, nil
}

func (s *Server) DialInternal(ctx context.Context, node *protocol.Node) (net.Conn, error) {
	if node.GetAddress() == "" || node.GetUnknown() {
		return nil, transport.ErrNoDirect
	}
	return s.ChordTransport.DialStream(ctx, node, protocol.Stream_INTERNAL)
}

func (s *Server) DialClient(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	ret, err := s.lookupRoutes(ctx, link.GetHostname(), nil)
	if err != nil {
		return nil, err
	}

	var isNoRoute bool
	for attempt := 0; attempt < 2; attempt++ {
		if ret.err != nil {
			return nil, ret.err
		}
		for _, route := range ret.routes {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			conn, err := s.getConn(ctx, route, link)
			if err == nil {
				return conn, nil
			}
			if tun.IsNoDirect(err) {
				isNoRoute = true
			} else {
				s.Logger.Error("Failed to establish connection to client",
					zap.String("hostname", link.GetHostname()),
					zap.Object("chord", route.GetChordDestination()),
					zap.Object("tunnel", route.GetTunnelDestination()),
					zap.Object("client", route.GetClientDestination()),
					zap.Error(err),
				)
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt == 1 {
			break
		}
		fresh, err := s.lookupRoutes(ctx, link.GetHostname(), ret)
		if err != nil {
			return nil, err
		}
		if fresh == ret {
			break
		}
		ret = fresh
	}

	if isNoRoute {
		return nil, tun.ErrTunnelClientNotConnected
	}

	return nil, tun.ErrDestinationNotFound // fallback to not found
}

func (s *Server) dialDirect(ctx context.Context, route *protocol.TunnelRoute) (net.Conn, error) {
	client := route.GetClientDestination()
	if tun.IsSessionAlias(client.GetAddress()) {
		return s.sessions.dial(client.GetAddress(), route.GetHostname())
	}
	return s.TunnelTransport.DialStream(ctx, client, protocol.Stream_DIRECT)
}
