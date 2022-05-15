package server

import (
	"context"
	"fmt"
	"strconv"

	"specter/spec/chord"
	"specter/spec/protocol"
	"specter/spec/rpc"
	"specter/spec/tun"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var _ rpc.RPCHandler = (*Server)(nil).handleRPC

func (s *Server) handleRPC(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	resp := &protocol.RPC_Response{}

	switch req.GetKind() {
	case protocol.RPC_GET_NODES:
		vnodes := make([]chord.VNode, tun.NumRedundantLinks)
		vnodes[0] = s.chord
		successors, _ := s.chord.GetSuccessors()
		copy(vnodes[1:], successors)

		servers := make([]*protocol.Node, len(vnodes))
		for i, vnode := range vnodes {
			if vnode == nil {
				continue
			}
			servers[i] = vnode.Identity()
		}
		resp.GetNodesResponse = &protocol.GetNodesResponse{
			Nodes: servers,
		}

	case protocol.RPC_PUBLISH_TUNNEL:
		hostname := "TODO: hostname generation"
		for k, server := range req.GetPublishTunnelRequest().GetServers() {
			bundle := &protocol.Tunnel{
				Client: req.GetPublishTunnelRequest().GetClient(),
				Server: server,
			}
			key := hostname + "/" + strconv.FormatInt(int64(k), 10)
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
