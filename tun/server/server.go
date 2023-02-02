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

type Server struct {
	logger          *zap.Logger
	chord           chord.VNode
	tunnelTransport transport.Transport
	chordTransport  transport.Transport
	rpcAcceptor     *acceptor.HTTP2Acceptor
	rootDomain      string
}

var _ tun.Server = (*Server)(nil)

func New(logger *zap.Logger, local chord.VNode, tunnelTrans transport.Transport, chordTrans transport.Transport, rootDomain string) *Server {
	return &Server{
		logger:          logger,
		chord:           local,
		tunnelTransport: tunnelTrans,
		chordTransport:  chordTrans,
		rootDomain:      rootDomain,
		rpcAcceptor:     acceptor.NewH2Acceptor(nil),
	}
}

func (s *Server) Identity() *protocol.Node {
	return s.tunnelTransport.Identity()
}

func (s *Server) AttachRouter(ctx context.Context, router *transport.StreamRouter) {
	router.HandleChord(protocol.Stream_PROXY, func(delegate *transport.StreamDelegate) {
		l := s.logger.With(
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
	s.logger.Info("publishing identities to chord")

	publishCtx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	if err := s.publishIdentities(publishCtx); err != nil {
		s.logger.Panic("publishing identities pair", zap.Error(err))
	}

	s.logger.Info("specter server started")
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

	bundle := &protocol.Tunnel{}
	err = rpc.Receive(delegation, bundle)
	if err != nil {
		s.logger.Error("receiving remote tunnel negotiation", zap.Error(err))
		return
	}

	l := s.logger.With(
		zap.String("hostname", bundle.GetHostname()),
		zap.Uint64("client", bundle.GetClient().GetId()),
	)
	l.Debug("received proxy stream from remote node",
		zap.Object("remote_chord", delegation.Identity),
		zap.Object("chord", bundle.GetChord()),
		zap.Object("tun", bundle.GetTun()))

	if bundle.GetTun().GetId() != s.tunnelTransport.Identity().GetId() {
		l.Warn("received remote connection for the wrong server",
			zap.Uint64("expected", s.tunnelTransport.Identity().GetId()),
			zap.Uint64("got", bundle.GetTun().GetId()),
		)
		err = tun.ErrDestinationNotFound
		return
	}

	clientConn, err = s.tunnelTransport.DialStream(ctx, bundle.GetClient(), protocol.Stream_DIRECT)
	if err != nil && !tun.IsNoDirect(err) {
		l.Error("dialing connection to connected client", zap.Error(err))
	}
}

func (s *Server) getConn(ctx context.Context, bundle *protocol.Tunnel) (net.Conn, error) {
	l := s.logger.With(
		zap.String("hostname", bundle.GetHostname()),
		zap.Uint64("client", bundle.GetClient().GetId()),
	)

	if bundle.GetTun().GetId() == s.tunnelTransport.Identity().GetId() {
		l.Debug("client is connected to us, opening direct stream")

		return s.tunnelTransport.DialStream(ctx, bundle.GetClient(), protocol.Stream_DIRECT)
	} else {
		l.Debug("client is connected to remote node, opening proxy stream",
			zap.Object("chord", bundle.GetChord()),
			zap.Object("tun", bundle.GetTun()))

		conn, err := s.chordTransport.DialStream(ctx, bundle.GetChord(), protocol.Stream_PROXY)
		if err != nil {
			return nil, err
		}
		if err := rpc.Send(conn, bundle); err != nil {
			l.Error("sending remote tunnel negotiation", zap.Error(err))
			return nil, err
		}
		status := &protocol.TunnelStatus{}
		conn.SetReadDeadline(time.Now().Add(time.Second * 3))
		if err := rpc.Receive(conn, status); err != nil {
			l.Error("receiving remote tunnel status", zap.Error(err))
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
	if node.GetAddress() == "" || node.GetId() == 0 || node.GetUnknown() {
		return nil, transport.ErrNoDirect
	}
	return s.chordTransport.DialStream(ctx, node, protocol.Stream_INTERNAL)
}

// TODO: make routing selection more intelligent with rtt
func (s *Server) DialClient(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	isNoDirect := false
	for k := 1; k <= tun.NumRedundantLinks; k++ {
		key := tun.RoutingKey(link.GetHostname(), k)
		val, err := s.chord.Get(ctx, []byte(key))
		if err != nil {
			s.logger.Error("key lookup error", zap.String("key", key), zap.Error(err))
			continue
		}
		if val == nil {
			continue
		}
		bundle := &protocol.Tunnel{}
		if err := bundle.UnmarshalVT(val); err != nil {
			continue
		}
		clientConn, err := s.getConn(ctx, bundle)
		if err != nil {
			if tun.IsNoDirect(err) {
				isNoDirect = true
			} else {
				s.logger.Error("getting connection to client", zap.String("key", key), zap.Error(err))
			}
			continue
		}
		if err := rpc.Send(clientConn, link); err != nil {
			s.logger.Error("sending link information to client", zap.Error(err))
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

	s.unpublishIdentities(ctx)
}
