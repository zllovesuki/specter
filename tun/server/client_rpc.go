package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/zllovesuki/specter/rpc"
	"github.com/zllovesuki/specter/spec/chord"
	"github.com/zllovesuki/specter/spec/protocol"
	rpcSpec "github.com/zllovesuki/specter/spec/rpc"
	"github.com/zllovesuki/specter/spec/tun"

	"github.com/sethvargo/go-diceware/diceware"
	"go.uber.org/zap"
)

var _ rpcSpec.RPCHandler = (*Server)(nil).rpcHandler

var generator, _ = diceware.NewGenerator(nil)

func uniqueVNodes(nodes []chord.VNode) []chord.VNode {
	u := make([]chord.VNode, 0)
	m := make(map[uint64]bool)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if _, ok := m[node.ID()]; !ok {
			m[node.ID()] = true
			u = append(u, node)
		}
	}
	return u
}

func (s *Server) HandleRPC(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case delegate := <-s.clientTransport.RPC():
			conn := delegate.Connection
			l := s.logger.With(
				zap.Any("peer", delegate.Identity),
				zap.String("remote", conn.RemoteAddr().String()),
				zap.String("local", conn.LocalAddr().String()))
			l.Debug("New incoming RPC Stream")
			r := rpc.NewRPC(
				l.With(zap.String("pov", "client_rpc")),
				conn,
				s.rpcHandler)
			go r.Start(ctx)
		}
	}
}

func (s *Server) rpcHandler(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("server has shutdown")
	default:
	}

	resp := &protocol.RPC_Response{}

	switch req.GetKind() {
	case protocol.RPC_GET_NODES:
		vnodes := make([]chord.VNode, tun.NumRedundantLinks)
		vnodes[0] = s.chord
		successors, _ := s.chord.GetSuccessors()
		copy(vnodes[1:], successors)

		servers := make([]*protocol.Node, 0)
		for _, chord := range uniqueVNodes(vnodes) {
			identities, err := s.lookupIdentities(tun.IdentitiesChordKey(chord.Identity()))
			if err != nil {
				return nil, err
			}
			servers = append(servers, identities.GetTun())
		}
		resp.GetNodesResponse = &protocol.GetNodesResponse{
			Nodes: servers,
		}

	case protocol.RPC_PUBLISH_TUNNEL:
		requested := req.GetPublishTunnelRequest().GetServers()
		if len(requested) > tun.NumRedundantLinks {
			return nil, fmt.Errorf("too many requested endpoints")
		}

		hostname := strings.Join(generator.MustGenerate(5), "-")
		for k, server := range requested {
			identities, err := s.lookupIdentities(tun.IdentitiesTunKey(server))
			if err != nil {
				return nil, err
			}
			bundle := &protocol.Tunnel{
				Client:   req.GetPublishTunnelRequest().GetClient(),
				Chord:    identities.GetChord(),
				Tun:      identities.GetTun(),
				Hostname: hostname,
			}
			val, err := bundle.MarshalVT()
			if err != nil {
				return nil, err
			}
			key := tun.BundleKey(hostname, k+1)
			if err := s.chord.Put([]byte(key), val); err != nil {
				continue
			}
		}
		resp.PublishTunnelResponse = &protocol.PublishTunnelResponse{
			Hostname: strings.Join([]string{hostname, s.rootDomain}, "."),
		}

	default:
		s.logger.Warn("Unknown RPC Call", zap.String("kind", req.GetKind().String()))
		return nil, fmt.Errorf("unknown RPC call: %s", req.GetKind())
	}

	return resp, nil
}
