package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util/acceptor"

	"go.uber.org/zap"
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
	Chord           chord.VNode
	TunnelTransport transport.Transport
	ChordTransport  transport.Transport
	Resolver        tun.DNSResolver
	Apex            string
	Acme            string
}

type Server struct {
	rpcAcceptor *acceptor.HTTP2Acceptor
	Config
}

var _ tun.Server = (*Server)(nil)
var _ protocol.TunnelService = (*Server)(nil)

func New(cfg Config) *Server {
	return &Server{
		Config:      cfg,
		rpcAcceptor: acceptor.NewH2Acceptor(nil),
	}
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
	err = rpc.Receive(delegation, route)
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

	clientConn, err = s.TunnelTransport.DialStream(ctx, route.GetClientDestination(), protocol.Stream_DIRECT)
	if err != nil && !tun.IsNoDirect(err) {
		l.Error("Error dialing connection to connected client", zap.Error(err))
	}
}

func (s *Server) getConn(ctx context.Context, route *protocol.TunnelRoute) (net.Conn, error) {
	l := s.Logger.With(
		zap.String("hostname", route.GetHostname()),
		zap.Uint64("client", route.GetClientDestination().GetId()),
	)

	if route.GetTunnelDestination().String() == s.TunnelTransport.Identity().String() {
		l.Debug("client is connected to us, opening direct stream")

		return s.TunnelTransport.DialStream(ctx, route.GetClientDestination(), protocol.Stream_DIRECT)
	} else {
		l.Debug("client is connected to remote node, opening proxy stream",
			zap.Object("chord", route.GetChordDestination()),
			zap.Object("tunnel", route.GetTunnelDestination()))

		conn, err := s.ChordTransport.DialStream(ctx, route.GetChordDestination(), protocol.Stream_PROXY)
		if err != nil {
			return nil, err
		}
		if err := rpc.Send(conn, route); err != nil {
			l.Error("sending remote tunnel negotiation", zap.Error(err))
			return nil, err
		}
		status := &protocol.TunnelStatus{}
		conn.SetReadDeadline(time.Now().Add(time.Second * 3))
		if err := rpc.Receive(conn, status); err != nil {
			l.Error("Error receiving remote tunnel status", zap.Error(err))
			return nil, err
		}
		conn.SetReadDeadline(time.Time{})
		switch status.GetStatus() {
		case protocol.TunnelStatusCode_STATUS_OK:
			return conn, nil
		case protocol.TunnelStatusCode_NO_DIRECT:
			return nil, tun.ErrTunnelClientNotConnected
		default:
			return nil, fmt.Errorf(status.GetError())
		}
	}
}

func (s *Server) DialInternal(ctx context.Context, node *protocol.Node) (net.Conn, error) {
	if node.GetAddress() == "" || node.GetUnknown() {
		return nil, transport.ErrNoDirect
	}
	return s.ChordTransport.DialStream(ctx, node, protocol.Stream_INTERNAL)
}

// TODO: make routing selection more intelligent with rtt
func (s *Server) DialClient(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	isNoDirect := false
	for k := 1; k <= tun.NumRedundantLinks; k++ {
		key := tun.RoutingKey(link.GetHostname(), k)
		val, err := s.Chord.Get(ctx, []byte(key))
		if err != nil {
			s.Logger.Error("key lookup error", zap.String("key", key), zap.Error(err))
			continue
		}
		if val == nil {
			continue
		}
		route := &protocol.TunnelRoute{}
		if err := route.UnmarshalVT(val); err != nil {
			continue
		}
		clientConn, err := s.getConn(ctx, route)
		if err != nil {
			if tun.IsNoDirect(err) {
				isNoDirect = true
			} else {
				s.Logger.Error("getting connection to client", zap.String("key", key), zap.Error(err))
			}
			continue
		}
		if err := rpc.Send(clientConn, link); err != nil {
			s.Logger.Error("sending link information to client", zap.Error(err))
			clientConn.Close()
			continue
		}
		// TODO: optionally cache the routing information
		return clientConn, nil
	}

	if isNoDirect {
		return nil, tun.ErrTunnelClientNotConnected
	}

	return nil, tun.ErrDestinationNotFound
}

func (s *Server) Stop() {
	s.rpcAcceptor.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	s.unpublishDestinations(ctx)
}
