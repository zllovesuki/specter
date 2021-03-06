package server

import (
	"context"
	"net"

	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"

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

func (s *Server) Accept(ctx context.Context) {
	s.logger.Info("publishing identities to chord")
	if err := s.publishIdentities(); err != nil {
		s.logger.Fatal("publishing identities pair", zap.Error(err))
	}

	s.logger.Info("specter server started")

	for {
		select {
		case <-ctx.Done():
			return

		case delegate := <-s.chordTransport.Direct():
			go s.handleConn(ctx, delegate)

		case delegate := <-s.clientTransport.Direct():
			// client uses this to register connection
			// but it is a no-op on the server side
			delegate.Connection.Close()
		}
	}
}

func (s *Server) handleConn(ctx context.Context, delegation *transport.StreamDelegate) {
	var err error
	var clientConn net.Conn

	defer func() {
		if err != nil {
			delegation.Connection.Close()
		}
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
	clientConn, err = s.clientTransport.DialDirect(ctx, bundle.GetClient())
	if err != nil {
		l.Error("dialing connection to connected client")
		return
	}

	tun.Pipe(delegation.Connection, clientConn)
}

func (s *Server) getConn(ctx context.Context, bundle *protocol.Tunnel) (net.Conn, error) {
	l := s.logger.With(
		zap.String("hostname", bundle.GetHostname()),
		zap.Uint64("client", bundle.GetClient().GetId()),
	)
	if bundle.GetTun().GetId() == s.clientTransport.Identity().GetId() {
		l.Debug("client is connected to us, opening direct stream")

		return s.clientTransport.DialDirect(ctx, bundle.GetClient())
	} else {
		l.Debug("client is connected to remote node, opening proxy stream",
			zap.Uint64("chord", bundle.GetChord().GetId()),
			zap.Uint64("tun", bundle.GetTun().GetId()))

		conn, err := s.chordTransport.DialDirect(ctx, bundle.GetChord())
		if err != nil {
			return nil, err
		}
		if err := rpc.Send(conn, bundle); err != nil {
			l.Error("sending remote tunnel negotiation", zap.Error(err))
			return nil, err
		}
		return conn, nil
	}
}

func (s *Server) Dial(ctx context.Context, link *protocol.Link) (net.Conn, error) {
	for k := 1; k <= tun.NumRedundantLinks; k++ {
		key := tun.BundleKey(link.GetHostname(), k)
		val, err := s.chord.Get([]byte(key))
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
			s.logger.Error("getting connection to client", zap.String("key", key), zap.Error(err))
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

	return nil, tun.ErrDestinationNotFound
}

func (s *Server) Stop() {
	s.unpublishIdentities()
}
