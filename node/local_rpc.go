package node

import (
	"context"
	"fmt"

	"specter/rpc"
	"specter/spec/protocol"

	"go.uber.org/zap"
)

var _ rpc.RPCHandler = (*LocalNode)(nil).rpcHandler

func (n *LocalNode) HandleRPC() {
	for {
		select {
		case s := <-n.conf.Transport.RPC():
			n.logger.Debug("New incoming peer RPC Stream", zap.String("remote", s.Remote.String()))
			xd := rpc.NewRPC(n.logger.With(zap.String("addr", s.Remote.String()), zap.String("pov", "local_rpc")), s.Connection, n.rpcHandler)
			go xd.Start(n.stopCtx)
		case <-n.stopCtx.Done():
			return
		}
	}
}

func (n *LocalNode) rpcHandler(ctx context.Context, rr *protocol.RequestReply) error {
	if rr.GetType() != protocol.RequestReply_REQUEST {
		return fmt.Errorf("incoming RPC is not a Request: %s", rr.GetType())
	}

	rr.Type = protocol.RequestReply_REPLY

	// n.logger.Debug("Incoming RPC Request", zap.Any("rr", rr))

	switch rr.GetKind() {
	case protocol.RequestReply_IDENTITY:
		rr.IdentityRequest = nil
		rr.IdentityResponse = &protocol.IdentityResponse{
			Identity: n.Identity(),
		}

	case protocol.RequestReply_PING:
		rr.PingRequest = nil
		rr.PingResponse = &protocol.PingResponse{}

	case protocol.RequestReply_NOTIFY:
		predecessor := rr.GetNotifyRequest().GetPredecessor()
		vnode, err := NewRemoteNode(ctx, n.conf.Transport, n.conf.Logger, predecessor)
		if err != nil {
			return err
		}
		n.Notify(vnode)

		rr.NotifyRequest = nil
		rr.NotifyResponse = &protocol.NotifyResponse{}

	case protocol.RequestReply_FIND_SUCCESSOR:
		key := rr.GetFindSuccessorRequest().GetKey()
		vnode, err := n.FindSuccessor(key)
		if err != nil {
			return err
		}
		rr.FindSuccessorRequest = nil
		rr.FindSuccessorResponse = &protocol.FindSuccessorResponse{
			Successor: vnode.Identity(),
		}

	case protocol.RequestReply_GET_SUCCESSORS:
		vnodes, err := n.GetSuccessors()
		if err != nil {
			return err
		}
		identities := make([]*protocol.Node, 0, len(vnodes))
		for _, vnode := range vnodes {
			if vnode == nil {
				continue
			}
			identities = append(identities, vnode.Identity())
		}
		rr.GetSuccessorsRequest = nil
		rr.GetSuccessorsResponse = &protocol.GetSuccessorsResponse{
			Successors: identities,
		}

	case protocol.RequestReply_GET_PREDECESSOR:
		vnode, err := n.GetPredecessor()
		if err != nil {
			return err
		}
		var pre *protocol.Node
		if vnode != nil {
			pre = vnode.Identity()
		}
		rr.GetPredecessorRequest = nil
		rr.GetPredecessorResponse = &protocol.GetPredecessorResponse{
			Predecessor: pre,
		}

	case protocol.RequestReply_KV:
		req := rr.GetKvRequest()
		resp := &protocol.KVResponse{}

		switch req.GetOp() {
		case protocol.KVOperation_GET:
			val, err := n.Get(req.GetKey())
			if err != nil {
				return err
			}
			resp.Value = val
		case protocol.KVOperation_PUT:
			if err := n.Put(req.GetKey(), req.GetValue()); err != nil {
				return err
			}
		case protocol.KVOperation_DELETE:
			if err := n.Delete(req.GetKey()); err != nil {
				return err
			}
		default:
			n.logger.Warn("Unknown KV Operation", zap.String("Op", req.GetOp().String()))
			return fmt.Errorf("unknown KV Operation: %s", req.GetOp())
		}

		rr.KvRequest = nil
		rr.KvResponse = resp
	default:
		n.logger.Warn("Unknown RPC Call", zap.String("kind", rr.GetKind().String()))
		return fmt.Errorf("unknown RPC call: %s", rr.GetKind())
	}

	// n.logger.Debug("Responding to RPC Request", zap.Any("rr", rr))

	return nil
}
