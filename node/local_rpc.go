package node

import (
	"context"
	"errors"

	"specter/overlay"
	"specter/spec/protocol"

	"go.uber.org/zap"
)

func (n *LocalNode) HandleRPC() {
	for {
		select {
		case r := <-n.conf.Transport.RPC():
			n.logger.Debug("New incoming RPC Stream")
			xd := overlay.NewRPC(r, n.handleRequest)
			go xd.Start(n.stopCtx)
		case <-n.stopCtx.Done():
			return
		}
	}
}

func (n *LocalNode) handleRequest(ctx context.Context, rr *protocol.RequestReply) error {
	if rr.GetType() != protocol.RequestReply_REQUEST {
		return errors.New("incoming RPC is not a Request")
	}

	n.logger.Debug("Incoming RPC Request", zap.String("kind", rr.GetKind().String()))

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
		vnode, err := NewKnownRemoteNode(ctx, n.conf.Transport, predecessor)
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
			return errors.New("unknown KV Operation")
		}

		rr.KvRequest = nil
		rr.KvResponse = resp
	default:
		return errors.New("unknown RPC call")
	}

	n.logger.Debug("Responding to RPC Request", zap.String("kind", rr.GetKind().String()))

	rr.Type = protocol.RequestReply_REPLY
	return nil
}
