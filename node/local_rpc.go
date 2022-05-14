package node

import (
	"context"
	"fmt"

	"specter/rpc"
	"specter/spec"
	"specter/spec/protocol"

	"go.uber.org/zap"
)

var _ spec.RPCHandler = (*LocalNode)(nil).rpcHandler

func (n *LocalNode) HandleRPC() {
	for {
		select {
		case s := <-n.conf.Transport.RPC():
			n.logger.Debug("New incoming peer RPC Stream", zap.String("remote", s.RemoteAddr().String()), zap.String("local", s.LocalAddr().String()))
			xd := rpc.NewRPC(
				n.logger.With(
					zap.String("remote", s.RemoteAddr().String()),
					zap.String("local", s.LocalAddr().String()),
					zap.String("pov", "local_rpc")),
				s,
				n.rpcHandler)
			go xd.Start(n.stopCtx)
		case <-n.stopCtx.Done():
			return
		}
	}
}

func (n *LocalNode) rpcHandler(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	resp := &protocol.RPC_Response{}

	switch req.GetKind() {
	case protocol.RPC_IDENTITY:
		resp.IdentityResponse = &protocol.IdentityResponse{
			Identity: n.Identity(),
		}

	case protocol.RPC_PING:
		resp.PingResponse = &protocol.PingResponse{}

	case protocol.RPC_NOTIFY:
		predecessor := req.GetNotifyRequest().GetPredecessor()
		vnode, err := NewRemoteNode(ctx, n.conf.Transport, n.conf.Logger, predecessor)
		if err != nil {
			return nil, err
		}

		if err := n.Notify(vnode); err != nil {
			return nil, err
		}

		resp.NotifyResponse = &protocol.NotifyResponse{}

	case protocol.RPC_FIND_SUCCESSOR:
		key := req.GetFindSuccessorRequest().GetKey()
		vnode, err := n.FindSuccessor(key)
		if err != nil {
			return nil, err
		}
		resp.FindSuccessorResponse = &protocol.FindSuccessorResponse{
			Successor: vnode.Identity(),
		}

	case protocol.RPC_GET_SUCCESSORS:
		vnodes, err := n.GetSuccessors()
		if err != nil {
			return nil, err
		}
		identities := make([]*protocol.Node, 0, len(vnodes))
		for _, vnode := range vnodes {
			if vnode == nil {
				continue
			}
			identities = append(identities, vnode.Identity())
		}
		resp.GetSuccessorsResponse = &protocol.GetSuccessorsResponse{
			Successors: identities,
		}

	case protocol.RPC_GET_PREDECESSOR:
		vnode, err := n.GetPredecessor()
		if err != nil {
			return nil, err
		}
		var pre *protocol.Node
		if vnode != nil {
			pre = vnode.Identity()
		}
		resp.GetPredecessorResponse = &protocol.GetPredecessorResponse{
			Predecessor: pre,
		}

	case protocol.RPC_KV:
		kvReq := req.GetKvRequest()
		kvResp := &protocol.KVResponse{}

		switch kvReq.GetOp() {
		case protocol.KVOperation_GET:
			val, err := n.Get(kvReq.GetKey())
			if err != nil {
				return nil, err
			}
			kvResp.Value = val
		case protocol.KVOperation_PUT:
			if err := n.Put(kvReq.GetKey(), kvReq.GetValue()); err != nil {
				return nil, err
			}
		case protocol.KVOperation_DELETE:
			if err := n.Delete(kvReq.GetKey()); err != nil {
				return nil, err
			}
		case protocol.KVOperation_FIND_KEYS:
			keys, err := n.FindKeys(kvReq.GetLowKey(), kvReq.GetHighKey())
			if err != nil {
				return nil, err
			}
			kvResp.Keys = keys
		default:
			n.logger.Warn("Unknown KV Operation", zap.String("Op", kvReq.GetOp().String()))
			return nil, fmt.Errorf("unknown KV Operation: %s", kvReq.GetOp())
		}

		resp.KvResponse = kvResp
	default:
		n.logger.Warn("Unknown RPC Call", zap.String("kind", req.GetKind().String()))
		return nil, fmt.Errorf("unknown RPC call: %s", req.GetKind())
	}

	return resp, nil
}
