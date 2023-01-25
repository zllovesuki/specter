package server

import (
	"context"
	"fmt"
	"net"
	"time"

	rpcImpl "kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util/router"

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
	clientTransport transport.Transport
	chordTransport  transport.Transport
	rootDomain      string
}

var _ tun.Server = (*Server)(nil)

func New(logger *zap.Logger, local chord.VNode, clientTrans transport.Transport, chordTrans transport.Transport, rootDomain string) *Server {
	return &Server{
		logger:          logger,
		chord:           local,
		clientTransport: clientTrans,
		chordTransport:  chordTrans,
		rootDomain:      rootDomain,
	}
}

func (s *Server) AttachRouter(ctx context.Context, router *router.StreamRouter) {
	rpcServer := rpcImpl.NewRPC(ctx, s.logger.With(zap.String("pov", "client_rpc")), nil)
	rpcServer.LimitMessageSize(2048)

	router.AttachChord(protocol.Stream_PROXY, func(delegate *transport.StreamDelegate) {
		s.handleProxyConn(ctx, delegate)
	})
	router.AttachClient(protocol.Stream_DIRECT, func(delegate *transport.StreamDelegate) {
		// client uses this to register connection
		// but it is a no-op on the server side
		delegate.Connection.Close()
	})
	router.AttachClient(protocol.Stream_RPC, func(delegate *transport.StreamDelegate) {
		defer delegate.Connection.Close()

		l := s.logger.With(
			zap.Any("peer", delegate.Identity),
			zap.String("remote", delegate.Connection.RemoteAddr().String()),
			zap.String("local", delegate.Connection.LocalAddr().String()),
		)
		if err := rpcServer.HandleRequest(ctx, delegate.Connection, s.rpcHandlerMiddlerware(delegate.Identity)); err != nil {
			l.Error("Error handling RPC request", zap.Error(err))
		}
	})
}

func (s *Server) Start(ctx context.Context) {
	s.logger.Info("publishing identities to chord")

	publishCtx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	if err := s.publishIdentities(publishCtx); err != nil {
		s.logger.Fatal("publishing identities pair", zap.Error(err))
	}

	s.logger.Info("specter server started")
}

func (s *Server) handleProxyConn(ctx context.Context, delegation *transport.StreamDelegate) {
	var err error
	var clientConn net.Conn

	defer func() {
		tun.SendStatusProto(delegation.Connection, err)
		if err != nil {
			delegation.Connection.Close()
			return
		}
		tun.Pipe(delegation.Connection, clientConn)
	}()

	bundle := &protocol.Tunnel{}
	err = rpc.Receive(delegation.Connection, bundle)
	if err != nil {
		s.logger.Error("receiving remote tunnel negotiation", zap.Error(err))
		return
	}
	l := s.logger.With(
		zap.String("hostname", bundle.GetHostname()),
		zap.Uint64("client", bundle.GetClient().GetId()),
	)
	l.Debug("received proxy stream from remote node",
		zap.Uint64("remote_chord", delegation.Identity.GetId()),
		zap.Uint64("chord", bundle.GetChord().GetId()),
		zap.Uint64("tun", bundle.GetTun().GetId()))

	if bundle.GetTun().GetId() != s.clientTransport.Identity().GetId() {
		l.Warn("received remote connection for the wrong server",
			zap.Uint64("expected", s.clientTransport.Identity().GetId()),
			zap.Uint64("got", bundle.GetTun().GetId()),
		)
		err = tun.ErrDestinationNotFound
		return
	}

	clientConn, err = s.clientTransport.DialStream(ctx, bundle.GetClient(), protocol.Stream_DIRECT)
	if err != nil && !tun.IsNoDirect(err) {
		l.Error("dialing connection to connected client", zap.Error(err))
	}
}

func (s *Server) getConn(ctx context.Context, bundle *protocol.Tunnel) (net.Conn, error) {
	l := s.logger.With(
		zap.String("hostname", bundle.GetHostname()),
		zap.Uint64("client", bundle.GetClient().GetId()),
	)
	if bundle.GetTun().GetId() == s.clientTransport.Identity().GetId() {
		l.Debug("client is connected to us, opening direct stream")

		return s.clientTransport.DialStream(ctx, bundle.GetClient(), protocol.Stream_DIRECT)
	} else {
		l.Debug("client is connected to remote node, opening proxy stream",
			zap.Uint64("chord", bundle.GetChord().GetId()),
			zap.Uint64("tun", bundle.GetTun().GetId()))

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

// TODO: make routing selection more intelligent
func (s *Server) Dial(ctx context.Context, link *protocol.Link) (net.Conn, error) {
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	s.unpublishIdentities(ctx)
}
