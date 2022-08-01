package chord

import (
	"context"
	"fmt"

	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	rpcSpec "kon.nect.sh/specter/spec/rpc"

	"go.uber.org/zap"
)

var _ rpcSpec.RPCHandler = (*LocalNode)(nil).rpcHandler

func (n *LocalNode) HandleRPC(ctx context.Context) {
	for {
		select {
		case delegate := <-n.Transport.RPC():
			s := delegate.Connection
			l := n.Logger.With(
				zap.Any("peer", delegate.Identity),
				zap.String("remote", s.RemoteAddr().String()),
				zap.String("local", s.LocalAddr().String()))
			l.Debug("New incoming RPC Stream")
			r := rpc.NewRPC(
				l.With(zap.String("pov", "local_rpc")),
				s,
				n.rpcHandler)
			go r.Start(ctx)

		case <-n.stopCh:
			return
		}
	}
}

func (n *LocalNode) rpcHandler(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	if !n.isRunning.Load() {
		return nil, chord.ErrNodeGone
	}

	resp := &protocol.RPC_Response{}
	var vnode chord.VNode
	var err error

	switch req.GetKind() {
	case protocol.RPC_IDENTITY:
		resp.IdentityResponse = &protocol.IdentityResponse{
			Identity: n.Identity(),
		}

	case protocol.RPC_PING:
		resp.PingResponse = &protocol.PingResponse{}
		if err := n.Ping(); err != nil {
			return nil, err
		}

	case protocol.RPC_NOTIFY:
		resp.NotifyResponse = &protocol.NotifyResponse{}
		predecessor := req.GetNotifyRequest().GetPredecessor()

		vnode, err = createRPC(ctx, n.Transport, n.Logger, predecessor)
		if err != nil {
			return nil, err
		}

		err = n.Notify(vnode)
		if err != nil {
			return nil, err
		}

	case protocol.RPC_FIND_SUCCESSOR:
		key := req.GetFindSuccessorRequest().GetKey()
		vnode, err = n.FindSuccessor(key)
		if err != nil {
			return nil, err
		}
		resp.FindSuccessorResponse = &protocol.FindSuccessorResponse{
			Successor: vnode.Identity(),
		}

	case protocol.RPC_GET_SUCCESSORS:
		var vnodes []chord.VNode

		vnodes, err = n.GetSuccessors()
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
		vnode, err = n.GetPredecessor()
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

		case protocol.KVOperation_DIRECT_PUTS:
			if err := n.DirectPuts(kvReq.GetKeys(), kvReq.GetValues()); err != nil {
				return nil, err
			}
		default:
			n.Logger.Warn("Unknown KV Operation", zap.String("Op", kvReq.GetOp().String()))
			return nil, fmt.Errorf("unknown KV Operation: %s", kvReq.GetOp())
		}

		resp.KvResponse = kvResp
	default:
		n.Logger.Warn("Unknown RPC Call", zap.String("kind", req.GetKind().String()))
		return nil, fmt.Errorf("unknown RPC call: %s", req.GetKind())
	}

	return resp, nil
}
