package server

import (
	"context"
	"fmt"

	"specter/spec/protocol"
	"specter/spec/rpc"

	"go.uber.org/zap"
)

var _ rpc.RPCHandler = (*Server)(nil).handleRPC

func (s *Server) handleRPC(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	resp := &protocol.RPC_Response{}

	switch req.GetKind() {
	case protocol.RPC_GET_NODES:
	case protocol.RPC_PUBLISH_TUNNEL:
	default:
		s.logger.Warn("Unknown RPC Call", zap.String("kind", req.GetKind().String()))
		return nil, fmt.Errorf("unknown RPC call: %s", req.GetKind())
	}

	return resp, nil
}
