package chord

import (
	"context"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	rpcTimeout  = time.Second * 10
	pingTimeout = time.Second * 3
)

type RemoteNode struct {
	parentCtx context.Context

	logger *zap.Logger

	id  *protocol.Node
	rpc rpc.RPC
}

var _ chord.VNode = (*RemoteNode)(nil)

func NewRemoteNode(ctx context.Context, logger *zap.Logger, rpcClient rpc.RPC, peer *protocol.Node) (*RemoteNode, error) {
	n := &RemoteNode{
		parentCtx: ctx,
		id:        peer,
		logger:    logger,
		rpc:       rpcClient,
	}

	if peer.GetUnknown() {
		if err := n.handshake(rpcClient); err != nil {
			return nil, err
		}
	}
	return n, nil
}

func (n *RemoteNode) doRequest(ctx context.Context, timeout time.Duration, k protocol.RPC_Kind, modifiers ...func(rReq *protocol.RPC_Request)) (*protocol.RPC_Response, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rReq := newReqWithCtx(ctx, k)
	for _, modifier := range modifiers {
		modifier(rReq)
	}

	rResp, err := n.rpc.Call(ctx, n.id, rReq)
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote RPC error", zap.String("kind", k.String()), zap.String("peer", n.Identity().String()), zap.Error(err))
		}
		return nil, err
	}

	return rResp, nil
}

func (n *RemoteNode) handshake(r rpc.RPC) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_IDENTITY)
	rResp, err := r.Call(ctx, n.id, rReq)
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote Identity RPC", zap.String("node", n.Identity().String()), zap.Error(err))
		}
		return err
	}

	n.id = rResp.GetIdentityResponse().GetIdentity()
	return nil
}

func (n *RemoteNode) ID() uint64 {
	return n.id.GetId()
}

func (n *RemoteNode) Identity() *protocol.Node {
	return n.id
}

func newReq(t protocol.RPC_Kind) *protocol.RPC_Request {
	return &protocol.RPC_Request{
		Kind: t,
	}
}

func newReqWithCtx(ctx context.Context, t protocol.RPC_Kind) *protocol.RPC_Request {
	return &protocol.RPC_Request{
		Kind:           t,
		RequestContext: chord.GetRequestContext(ctx),
	}
}

func (n *RemoteNode) Ping() error {
	_, err := n.doRequest(n.parentCtx, pingTimeout, protocol.RPC_PING)
	return err
}

func (n *RemoteNode) Notify(predecessor chord.VNode) error {
	_, err := n.doRequest(n.parentCtx, rpcTimeout, protocol.RPC_NOTIFY, func(rReq *protocol.RPC_Request) {
		rReq.NotifyRequest = &protocol.NotifyRequest{
			Predecessor: predecessor.Identity(),
		}
	})
	return err
}

func (n *RemoteNode) FindSuccessor(key uint64) (chord.VNode, error) {
	rResp, err := n.doRequest(n.parentCtx, rpcTimeout, protocol.RPC_FIND_SUCCESSOR, func(rReq *protocol.RPC_Request) {
		rReq.FindSuccessorRequest = &protocol.FindSuccessorRequest{
			Key: key,
		}
	})
	if err != nil {
		return nil, err
	}

	resp := rResp.GetFindSuccessorResponse()
	if resp.GetSuccessor() == nil {
		return nil, nil
	}

	if resp.GetSuccessor().GetId() == n.ID() {
		return n, nil
	}

	succ, err := createRPC(n.parentCtx, n.logger, n.rpc, resp.GetSuccessor())
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("creating new RemoteNode", zap.String("peer", resp.GetSuccessor().String()), zap.Error(err))
		}
		return nil, err
	}

	return succ, nil
}

func (n *RemoteNode) GetSuccessors() ([]chord.VNode, error) {
	rResp, err := n.doRequest(n.parentCtx, rpcTimeout, protocol.RPC_GET_SUCCESSORS)
	if err != nil {
		return nil, err
	}

	resp := rResp.GetGetSuccessorsResponse()
	succList := resp.GetSuccessors()

	nodes := make([]chord.VNode, 0, len(succList))
	for _, succ := range succList {
		node, err := createRPC(n.parentCtx, n.logger, n.rpc, succ)
		if err != nil {
			if !chord.ErrorIsRetryable(err) {
				n.logger.Error("create RemoteNote in GetSuccessors", zap.String("peer", n.Identity().String()), zap.Error(err))
			}
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (n *RemoteNode) GetPredecessor() (chord.VNode, error) {
	rResp, err := n.doRequest(n.parentCtx, rpcTimeout, protocol.RPC_GET_PREDECESSOR)
	if err != nil {
		return nil, err
	}

	resp := rResp.GetGetPredecessorResponse()
	if resp.GetPredecessor() == nil {
		return nil, nil
	}
	if resp.GetPredecessor().GetId() == n.ID() {
		return n, nil
	}

	pre, err := createRPC(n.parentCtx, n.logger, n.rpc, resp.GetPredecessor())
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("creating new RemoteNode", zap.String("peer", resp.GetPredecessor().String()), zap.Error(err))
		}
		return nil, err
	}

	return pre, nil
}

func (n *RemoteNode) Put(ctx context.Context, key, value []byte) error {
	_, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:    protocol.KVOperation_SIMPLE_PUT,
			Key:   key,
			Value: value,
		}
	})
	return err
}

func (n *RemoteNode) Get(ctx context.Context, key []byte) ([]byte, error) {
	rResp, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:  protocol.KVOperation_SIMPLE_GET,
			Key: key,
		}
	})
	if err != nil {
		return nil, err
	}
	return rResp.GetKvResponse().GetValue(), nil
}

func (n *RemoteNode) Delete(ctx context.Context, key []byte) error {
	_, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:  protocol.KVOperation_SIMPLE_DELETE,
			Key: key,
		}
	})
	return err
}

func (n *RemoteNode) PrefixAppend(ctx context.Context, prefix []byte, child []byte) error {
	_, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:    protocol.KVOperation_PREFIX_APPEND,
			Key:   prefix,
			Value: child,
		}
	})
	return err
}

func (n *RemoteNode) PrefixList(ctx context.Context, prefix []byte) ([][]byte, error) {
	rResp, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:  protocol.KVOperation_PREFIX_LIST,
			Key: prefix,
		}
	})
	if err != nil {
		return nil, err
	}
	return rResp.GetKvResponse().GetKeys(), nil
}

func (n *RemoteNode) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	rResp, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:    protocol.KVOperation_PREFIX_CONTAINS,
			Key:   prefix,
			Value: child,
		}
	})
	if err != nil {
		return false, err
	}
	return rResp.GetKvResponse().GetValue() != nil, nil
}

func (n *RemoteNode) PrefixRemove(ctx context.Context, prefix []byte, child []byte) error {
	_, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:    protocol.KVOperation_PREFIX_REMOVE,
			Key:   prefix,
			Value: child,
		}
	})
	return err
}

func (n *RemoteNode) Acquire(ctx context.Context, lease []byte, ttl time.Duration) (uint64, error) {
	rResp, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:  protocol.KVOperation_LEASE_ACQUIRE,
			Key: lease,
			Lease: &protocol.KVLease{
				Ttl: durationpb.New(ttl),
			},
		}
	})
	if err != nil {
		return 0, err
	}
	return rResp.GetKvResponse().GetLease().GetToken(), nil
}

func (n *RemoteNode) Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	rResp, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:  protocol.KVOperation_LEASE_RENEWAL,
			Key: lease,
			Lease: &protocol.KVLease{
				Ttl:   durationpb.New(ttl),
				Token: prevToken,
			},
		}
	})
	if err != nil {
		return 0, err
	}
	return rResp.GetKvResponse().GetLease().GetToken(), nil
}

func (n *RemoteNode) Release(ctx context.Context, lease []byte, token uint64) error {
	_, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:  protocol.KVOperation_LEASE_RELEASE,
			Key: lease,
			Lease: &protocol.KVLease{
				Token: token,
			},
		}
	})
	return err
}

func (n *RemoteNode) Import(ctx context.Context, keys [][]byte, values []*protocol.KVTransfer) error {
	_, err := n.doRequest(ctx, rpcTimeout, protocol.RPC_KV, func(rReq *protocol.RPC_Request) {
		rReq.KvRequest = &protocol.KVRequest{
			Op:     protocol.KVOperation_IMPORT,
			Keys:   keys,
			Values: values,
		}
	})
	return err
}

func (n *RemoteNode) RequestToJoin(joiner chord.VNode) (chord.VNode, []chord.VNode, error) {
	rResp, err := n.doRequest(n.parentCtx, rpcTimeout, protocol.RPC_MEMBERSHIP_CHANGE, func(rReq *protocol.RPC_Request) {
		rReq.MembershipRequest = &protocol.MembershipChangeRequest{
			Op:     protocol.MembershipChangeOperation_JOIN_REQUEST,
			Joiner: joiner.Identity(),
		}
	})
	if err != nil {
		return nil, nil, err
	}

	resp := rResp.GetMembershipResponse()
	pre := resp.GetPredecessor()
	succList := resp.GetSuccessors()

	pp, err := createRPC(n.parentCtx, n.logger, n.rpc, pre)
	if err != nil {
		n.logger.Error("create Predecessor in RequestToJoin", zap.String("peer", n.Identity().String()), zap.Error(err))
		return nil, nil, err
	}

	nodes := make([]chord.VNode, 0, len(succList))
	for _, succ := range succList {
		node, err := createRPC(n.parentCtx, n.logger, n.rpc, succ)
		if err != nil {
			if !chord.ErrorIsRetryable(err) {
				n.logger.Error("create Successor in RequestToJoin", zap.String("peer", n.Identity().String()), zap.Error(err))
			}
			continue
		}
		nodes = append(nodes, node)
	}
	return pp, nodes, err
}

func (n *RemoteNode) FinishJoin(stablize bool, release bool) error {
	_, err := n.doRequest(n.parentCtx, rpcTimeout, protocol.RPC_MEMBERSHIP_CHANGE, func(rReq *protocol.RPC_Request) {
		rReq.MembershipRequest = &protocol.MembershipChangeRequest{
			Op:       protocol.MembershipChangeOperation_JOIN_FINISH,
			Stablize: stablize,
			Release:  release,
		}
	})
	return err
}

func (n *RemoteNode) RequestToLeave(leaver chord.VNode) error {
	_, err := n.doRequest(n.parentCtx, rpcTimeout, protocol.RPC_MEMBERSHIP_CHANGE, func(rReq *protocol.RPC_Request) {
		rReq.MembershipRequest = &protocol.MembershipChangeRequest{
			Op:     protocol.MembershipChangeOperation_LEAVE_REQUEST,
			Leaver: leaver.Identity(),
		}
	})
	return err
}

func (n *RemoteNode) FinishLeave(stablize bool, release bool) error {
	_, err := n.doRequest(n.parentCtx, rpcTimeout, protocol.RPC_MEMBERSHIP_CHANGE, func(rReq *protocol.RPC_Request) {
		rReq.MembershipRequest = &protocol.MembershipChangeRequest{
			Op:       protocol.MembershipChangeOperation_LEAVE_FINISH,
			Stablize: stablize,
			Release:  release,
		}
	})
	return err
}

func (n *RemoteNode) Stop() {
}
