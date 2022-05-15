package server

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"specter/rpc"
	"specter/spec/chord"
	"specter/spec/protocol"
	"specter/spec/transport"
	"specter/spec/tun"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
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
}

func New(logger *zap.Logger, local chord.VNode, clientTrans transport.Transport, chordTrans transport.Transport) *Server {
	return &Server{
		logger:          logger,
		chord:           local,
		clientTransport: clientTrans,
		chordTransport:  chordTrans,
	}
}

func (s *Server) Accept(ctx context.Context) {
	s.logger.Info("specter server started", zap.Uint64("server", s.clientTransport.Identity().GetId()))
	for {
		select {
		case <-ctx.Done():
			return

		case delegate := <-s.chordTransport.Direct():
			go s.handleConn(ctx, delegate.Connection)

		case delegate := <-s.clientTransport.Direct():
			// TODO: kill the entire connection because the client
			// should not be opening connection to us
			delegate.Connection.Close()

		case delegate := <-s.clientTransport.RPC():
			conn := delegate.Connection
			r := rpc.NewRPC(s.logger, conn, s.handleRPC)
			go r.Start(ctx)
		}
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	bundle := &protocol.Tunnel{}
	if err := rpc.Receive(conn, bundle); err != nil {
		return
	}
	if bundle.GetServer().GetId() != s.clientTransport.Identity().GetId() {
		return
	}
	clientConn, err := s.clientTransport.DialDirect(ctx, bundle.GetClient())
	if err != nil {
		return
	}
	tun.Pipe(conn, clientConn)
}

func (s *Server) getConn(ctx context.Context, bundle *protocol.Tunnel) (net.Conn, error) {
	if bundle.GetServer().GetId() == s.clientTransport.Identity().GetId() {
		return s.clientTransport.DialDirect(ctx, bundle.GetClient())
	} else {
		conn, err := s.chordTransport.DialDirect(ctx, bundle.GetServer())
		if err != nil {
			return nil, err
		}
		if err := rpc.Send(conn, bundle); err != nil {
			return nil, err
		}
		return conn, nil
	}
}

func (s *Server) Gateway(ctx context.Context, hostname string, visitor net.Conn) error {
	for k := int64(1); k < 3; k++ {
		key := hostname + "/" + strconv.FormatInt(k, 10)
		val, err := s.chord.Get([]byte(key))
		if err != nil {
			continue
		}
		if val == nil {
			continue
		}
		bundle := &protocol.Tunnel{}
		if err := proto.Unmarshal(val, bundle); err != nil {
			continue
		}
		if bundle.GetHostname() != hostname {
			return fmt.Errorf("hostname mismatch")
		}
		clientConn, err := s.getConn(ctx, bundle)
		if err != nil {
			return fmt.Errorf("cannot get connection to client")
		}

		// TODO: optionally cache the routing information
		go tun.Pipe(visitor, clientConn)

		return nil
	}
	return fmt.Errorf("tunnel not found")
}
