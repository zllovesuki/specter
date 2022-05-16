package server

import (
	"context"
	"fmt"
	"strings"

	"specter/spec/chord"
	"specter/spec/protocol"
	"specter/spec/rpc"
	"specter/spec/tun"

	"github.com/sethvargo/go-diceware/diceware"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var _ rpc.RPCHandler = (*Server)(nil).handleRPC

var generator, _ = diceware.NewGenerator(nil)

func (s *Server) handleRPC(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
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
		for _, chord := range vnodes {
			if chord == nil {
				continue
			}
			tunId, err := s.lookupIdentitiesByChord(chord.Identity())
			if err != nil {
				continue
			}
			servers = append(servers, tunId.GetTun())
		}

		resp.GetNodesResponse = &protocol.GetNodesResponse{
			Nodes: servers,
		}

	case protocol.RPC_PUBLISH_TUNNEL:
		hostname := strings.Join(generator.MustGenerate(6), "-")

		for k, server := range req.GetPublishTunnelRequest().GetServers() {
			identities, err := s.lookupIdentitiesByTun(server)
			if err != nil {
				return nil, err
			}
			bundle := &protocol.Tunnel{
				Client:   req.GetPublishTunnelRequest().GetClient(),
				Chord:    identities.GetChord(),
				Tun:      identities.GetTun(),
				Hostname: hostname,
			}
			key := tun.BundleKey(hostname, k+1)
			val, err := proto.Marshal(bundle)
			if err != nil {
				return nil, err
			}
			if err := s.chord.Put([]byte(key), val); err != nil {
				continue
			}
		}
		resp.PublishTunnelResponse = &protocol.PublishTunnelResponse{
			Hostname: hostname,
		}

	default:
		s.logger.Warn("Unknown RPC Call", zap.String("kind", req.GetKind().String()))
		return nil, fmt.Errorf("unknown RPC call: %s", req.GetKind())
	}

	return resp, nil
}
