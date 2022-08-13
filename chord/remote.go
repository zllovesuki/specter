package chord

import (
	"context"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"

	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	rpcTimeout  = time.Second * 10
	pingTimeout = time.Second * 5
)

type RemoteNode struct {
	parentCtx context.Context

	logger *zap.Logger

	id        *protocol.Node
	rpc       rpc.RPC
	transport transport.Transport
}

var _ chord.VNode = (*RemoteNode)(nil)

func NewRemoteNode(ctx context.Context, t transport.Transport, logger *zap.Logger, peer *protocol.Node) (*RemoteNode, error) {
	n := &RemoteNode{
		parentCtx: ctx,
		id:        peer,
		transport: t,
		logger:    logger,
	}
	var hs rpc.RPCHandshakeFunc
	if peer.GetUnknown() {
		hs = n.handshake
	}
	r, err := t.DialRPC(ctx, peer, hs)
	if err != nil {
		return nil, err
	}
	n.rpc = r
	return n, nil
}

func (n *RemoteNode) handshake(r rpc.RPC) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_IDENTITY)
	rResp, err := r.Call(ctx, rReq)
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

func (n *RemoteNode) Ping() error {
	ctx, cancel := context.WithTimeout(n.parentCtx, pingTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_PING)
	rReq.PingRequest = &protocol.PingRequest{}
	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote Ping RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) Notify(predecessor chord.VNode) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_NOTIFY)
	rReq.NotifyRequest = &protocol.NotifyRequest{
		Predecessor: predecessor.Identity(),
	}
	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote Notify RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) FindSuccessor(key uint64) (chord.VNode, error) {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_FIND_SUCCESSOR)
	rReq.FindSuccessorRequest = &protocol.FindSuccessorRequest{
		Key: key,
	}

	rResp, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote FindSuccessor RPC", zap.String("peer", n.Identity().String()), zap.Uint64("key", key), zap.Error(err))
		}
		return nil, err
	}

	resp := rResp.GetFindSuccessorResponse()
	if resp.GetSuccessor() == nil {
		return nil, nil
	}

	if resp.GetSuccessor().GetId() == n.ID() {
		return n, nil
	}

	succ, err := createRPC(n.parentCtx, n.transport, n.logger, resp.GetSuccessor())
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("creating new RemoteNode", zap.String("peer", resp.GetSuccessor().String()), zap.Error(err))
		}
		return nil, err
	}

	return succ, nil
}

func (n *RemoteNode) GetSuccessors() ([]chord.VNode, error) {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_GET_SUCCESSORS)
	rReq.GetSuccessorsRequest = &protocol.GetSuccessorsRequest{}

	rResp, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote GetSuccessors RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
		}
		return nil, err
	}

	resp := rResp.GetGetSuccessorsResponse()
	succList := resp.GetSuccessors()

	nodes := make([]chord.VNode, 0, len(succList))
	for _, succ := range succList {
		node, err := createRPC(n.parentCtx, n.transport, n.logger, succ)
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
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_GET_PREDECESSOR)
	rReq.GetPredecessorRequest = &protocol.GetPredecessorRequest{}

	rResp, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote GetPredecessor RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
		}
		return nil, err
	}

	resp := rResp.GetGetPredecessorResponse()
	if resp.GetPredecessor() == nil {
		return nil, nil
	}
	if resp.GetPredecessor().GetId() == n.ID() {
		return n, nil
	}

	pre, err := createRPC(n.parentCtx, n.transport, n.logger, resp.GetPredecessor())
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("creating new RemoteNode", zap.String("peer", resp.GetPredecessor().String()), zap.Error(err))
		}
		return nil, err
	}

	return pre, nil
}

func (n *RemoteNode) Put(key, value []byte) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:    protocol.KVOperation_SIMPLE_PUT,
		Key:   key,
		Value: value,
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote KV Put RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) Get(key []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_SIMPLE_GET,
		Key: key,
	}

	rResp, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote KV Get RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
		}
		return nil, err
	}
	return rResp.GetKvResponse().GetValue(), nil
}

func (n *RemoteNode) Delete(key []byte) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_SIMPLE_DELETE,
		Key: key,
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote KV Delete RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) PrefixAppend(prefix []byte, child []byte) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:    protocol.KVOperation_PREFIX_APPEND,
		Key:   prefix,
		Value: child,
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote KV PrefixAppend RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) PrefixList(prefix []byte) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_PREFIX_LIST,
		Key: prefix,
	}

	rResp, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote KV PrefixList RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
		}
		return nil, err
	}
	return rResp.GetKvResponse().GetKeys(), nil
}

func (n *RemoteNode) PrefixRemove(prefix []byte, child []byte) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:    protocol.KVOperation_PREFIX_REMOVE,
		Key:   prefix,
		Value: child,
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote KV PrefixRemove RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) Acquire(lease []byte, ttl time.Duration) (uint64, error) {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_LEASE_ACQUIRE,
		Key: lease,
		Lease: &protocol.KVLease{
			Ttl: durationpb.New(ttl),
		},
	}

	resp, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote KV Acquire RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
		}
		return 0, err
	}
	return resp.GetKvResponse().GetLease().GetToken(), nil
}

func (n *RemoteNode) Renew(lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_LEASE_RENEWAL,
		Key: lease,
		Lease: &protocol.KVLease{
			Ttl:   durationpb.New(ttl),
			Token: prevToken,
		},
	}

	resp, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote KV Renew RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
		}
		return 0, err
	}
	return resp.GetKvResponse().GetLease().GetToken(), nil
}

func (n *RemoteNode) Release(lease []byte, token uint64) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_LEASE_RELEASE,
		Key: lease,
		Lease: &protocol.KVLease{
			Token: token,
		},
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote KV Release RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) Import(keys [][]byte, values []*protocol.KVTransfer) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:     protocol.KVOperation_IMPORT,
		Keys:   keys,
		Values: values,
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote KV Import RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) RequestToJoin(joiner chord.VNode) (chord.VNode, []chord.VNode, error) {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_MEMBERSHIP_CHANGE)
	rReq.MembershipRequest = &protocol.MembershipChangeRequest{
		Op:     protocol.MembershipChangeOperation_JOIN_REQUEST,
		Joiner: joiner.Identity(),
	}

	rResp, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		if !chord.ErrorIsRetryable(err) {
			n.logger.Error("remote RequestToJoin RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
		}
		return nil, nil, err
	}

	resp := rResp.GetMembershipResponse()
	pre := resp.GetPredecessor()
	succList := resp.GetSuccessors()

	pp, err := createRPC(n.parentCtx, n.transport, n.logger, pre)
	if err != nil {
		n.logger.Error("create Predecessor in RequestToJoin", zap.String("peer", n.Identity().String()), zap.Error(err))
		return nil, nil, err
	}

	nodes := make([]chord.VNode, 0, len(succList))
	for _, succ := range succList {
		node, err := createRPC(n.parentCtx, n.transport, n.logger, succ)
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
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_MEMBERSHIP_CHANGE)
	rReq.MembershipRequest = &protocol.MembershipChangeRequest{
		Op:       protocol.MembershipChangeOperation_JOIN_FINISH,
		Stablize: stablize,
		Release:  release,
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote FinishJoin RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}

	return err
}

func (n *RemoteNode) RequestToLeave(leaver chord.VNode) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_MEMBERSHIP_CHANGE)
	rReq.MembershipRequest = &protocol.MembershipChangeRequest{
		Op:     protocol.MembershipChangeOperation_LEAVE_REQUEST,
		Leaver: leaver.Identity(),
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote RequestToLeave RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}

	return err
}

func (n *RemoteNode) FinishLeave(stablize bool, release bool) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_MEMBERSHIP_CHANGE)
	rReq.MembershipRequest = &protocol.MembershipChangeRequest{
		Op:       protocol.MembershipChangeOperation_LEAVE_FINISH,
		Stablize: stablize,
		Release:  release,
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil && !chord.ErrorIsRetryable(err) {
		n.logger.Error("remote FinishLeave RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}

	return err
}

func (n *RemoteNode) Stop() {
	n.rpc.Close()
}
