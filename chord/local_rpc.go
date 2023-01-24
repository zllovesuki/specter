package chord

import (
	"context"
	"fmt"

	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util/router"

	"go.uber.org/zap"
)

func (n *LocalNode) AttachRouter(ctx context.Context, router *router.StreamRouter) {
	rpcServer := rpc.NewRPC(ctx, n.Logger.With(zap.String("pov", "local_rpc")), nil)
	router.AttachChord(protocol.Stream_RPC, func(delegate *transport.StreamDelegate) {
		defer delegate.Connection.Close()

		l := n.Logger.With(
			zap.Any("peer", delegate.Identity),
			zap.String("remote", delegate.Connection.RemoteAddr().String()),
			zap.String("local", delegate.Connection.LocalAddr().String()),
		)

		if err := rpcServer.HandleRequest(ctx, delegate.Connection, n.rpcHandler); err != nil {
			l.Error("Error handling RPC request", zap.Error(err))
		}
	})
}

func (n *LocalNode) rpcHandler(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	if err := n.checkNodeState(false); err != nil {
		return nil, err
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

		vnode, err = createRPC(ctx, n.Logger, n.RPCClient, predecessor)
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

		identities := make([]*protocol.Node, 0)
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

	case protocol.RPC_MEMBERSHIP_CHANGE:
		chReq := req.GetMembershipRequest()
		chResp := &protocol.MembershipChangeResponse{}
		switch chReq.GetOp() {
		case protocol.MembershipChangeOperation_JOIN_REQUEST:
			joiner := chReq.GetJoiner()
			vnode, err = createRPC(ctx, n.Logger, n.RPCClient, joiner)
			if err != nil {
				return nil, err
			}

			pre, vnodes, err := n.RequestToJoin(vnode)
			if err != nil {
				return nil, err
			}
			chResp.Predecessor = pre.Identity()

			identities := make([]*protocol.Node, 0)
			for _, vnode := range vnodes {
				if vnode == nil {
					continue
				}
				identities = append(identities, vnode.Identity())
			}
			chResp.Successors = identities

		case protocol.MembershipChangeOperation_JOIN_FINISH:
			if err := n.FinishJoin(chReq.GetStablize(), chReq.GetRelease()); err != nil {
				return nil, err
			}

		case protocol.MembershipChangeOperation_LEAVE_REQUEST:
			leaver := chReq.GetLeaver()
			vnode, err = createRPC(ctx, n.Logger, n.RPCClient, leaver)
			if err != nil {
				return nil, err
			}
			if err := n.RequestToLeave(vnode); err != nil {
				return nil, err
			}

		case protocol.MembershipChangeOperation_LEAVE_FINISH:
			if err := n.FinishLeave(chReq.GetStablize(), chReq.GetRelease()); err != nil {
				return nil, err
			}

		default:
			n.Logger.Warn("Unknown Membership Change Operation", zap.String("Op", chReq.GetOp().String()))
			return nil, fmt.Errorf("unknown Membership Change Operation: %s", chReq.GetOp())
		}
		resp.MembershipResponse = chResp

	case protocol.RPC_KV:
		kvReq := req.GetKvRequest()
		kvResp := &protocol.KVResponse{}

		reqCtx := chord.WithRequestContext(ctx, req.GetRequestContext())

		switch kvReq.GetOp() {
		case protocol.KVOperation_SIMPLE_GET:
			val, err := n.Get(ctx, kvReq.GetKey())
			if err != nil {
				return nil, err
			}
			kvResp.Value = val
		case protocol.KVOperation_SIMPLE_PUT:
			if err := n.Put(reqCtx, kvReq.GetKey(), kvReq.GetValue()); err != nil {
				return nil, err
			}
		case protocol.KVOperation_SIMPLE_DELETE:
			if err := n.Delete(reqCtx, kvReq.GetKey()); err != nil {
				return nil, err
			}

		case protocol.KVOperation_PREFIX_APPEND:
			if err := n.PrefixAppend(reqCtx, kvReq.GetKey(), kvReq.GetValue()); err != nil {
				return nil, err
			}
		case protocol.KVOperation_PREFIX_LIST:
			val, err := n.PrefixList(reqCtx, kvReq.GetKey())
			if err != nil {
				return nil, err
			}
			kvResp.Keys = val
		case protocol.KVOperation_PREFIX_CONTAINS:
			b, err := n.PrefixContains(reqCtx, kvReq.GetKey(), kvReq.GetValue())
			if err != nil {
				return nil, err
			}
			if b {
				kvResp.Value = []byte{1}
			}
		case protocol.KVOperation_PREFIX_REMOVE:
			if err := n.PrefixRemove(reqCtx, kvReq.GetKey(), kvReq.GetValue()); err != nil {
				return nil, err
			}

		case protocol.KVOperation_LEASE_ACQUIRE:
			lease := kvReq.GetLease()
			token, err := n.Acquire(reqCtx, kvReq.GetKey(), lease.GetTtl().AsDuration())
			if err != nil {
				return nil, err
			}
			kvResp.Lease = &protocol.KVLease{
				Token: token,
			}
		case protocol.KVOperation_LEASE_RENEWAL:
			lease := kvReq.GetLease()
			token, err := n.Renew(reqCtx, kvReq.GetKey(), lease.GetTtl().AsDuration(), lease.GetToken())
			if err != nil {
				return nil, err
			}
			kvResp.Lease = &protocol.KVLease{
				Token: token,
			}
		case protocol.KVOperation_LEASE_RELEASE:
			lease := kvReq.GetLease()
			if err := n.Release(reqCtx, kvReq.GetKey(), lease.GetToken()); err != nil {
				return nil, err
			}

		case protocol.KVOperation_IMPORT:
			if err := n.Import(reqCtx, kvReq.GetKeys(), kvReq.GetValues()); err != nil {
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
