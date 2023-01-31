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
	baseContext context.Context
	baseLogger  *zap.Logger
	logger      *zap.Logger
	identity    *protocol.Node
	chordClient rpc.ChordClient
}

var _ chord.VNode = (*RemoteNode)(nil)

func NewRemoteNode(ctx context.Context, baseLogger *zap.Logger, chordClient rpc.ChordClient, peer *protocol.Node) (*RemoteNode, error) {
	if peer == nil {
		return nil, chord.ErrNodeNil
	}

	n := &RemoteNode{
		baseContext: ctx,
		baseLogger:  baseLogger,
		identity:    peer,
		chordClient: chordClient,
	}

	if peer.GetUnknown() {
		if err := n.getIdentity(n.chordClient, peer); err != nil {
			return nil, err
		}
	}
	n.logger = baseLogger.With(zap.String("component", "remoteNode"), zap.Object("node", n.identity))

	return n, nil
}

func (n *RemoteNode) getIdentity(r rpc.ChordClient, node *protocol.Node) error {
	if node.GetAddress() == "" {
		return chord.ErrNodeNil
	}

	ctx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, node), rpcTimeout)
	defer cancel()

	resp, err := r.Identity(ctx, &protocol.IdentityRequest{})
	if err != nil {
		n.baseLogger.Error("error obtaining Identity of RemoteNode", zap.Object("peer", node), zap.Error(err))
		return err
	}

	n.identity = resp.GetIdentity()
	return nil
}

func (n *RemoteNode) ID() uint64 {
	return n.identity.GetId()
}

func (n *RemoteNode) Identity() *protocol.Node {
	return n.identity
}

func (n *RemoteNode) Ping() error {
	ctx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, n.identity), pingTimeout)
	defer cancel()

	_, err := n.chordClient.Ping(ctx, &protocol.PingRequest{})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) Notify(predecessor chord.VNode) error {
	ctx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.Notify(ctx, &protocol.NotifyRequest{
		Predecessor: predecessor.Identity(),
	})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) FindSuccessor(key uint64) (chord.VNode, error) {
	ctx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, n.identity), rpcTimeout)
	defer cancel()

	resp, err := n.chordClient.FindSuccessor(ctx, &protocol.FindSuccessorRequest{
		Key: key,
	})
	if err != nil {
		return nil, chord.ErrorMapper(err)
	}

	if resp.GetSuccessor() == nil {
		return nil, nil
	}

	if resp.GetSuccessor().GetId() == n.ID() {
		return n, nil
	}

	succ, err := NewRemoteNode(n.baseContext, n.baseLogger, n.chordClient, resp.GetSuccessor())
	if err != nil {
		n.logger.Error("error creating new RemoteNode in FindSuccessor", zap.Object("peer", resp.GetSuccessor()), zap.Error(err))
		return nil, err
	}

	return succ, nil
}

func (n *RemoteNode) GetSuccessors() ([]chord.VNode, error) {
	ctx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, n.identity), rpcTimeout)
	defer cancel()

	resp, err := n.chordClient.GetSuccessors(ctx, &protocol.GetSuccessorsRequest{})
	if err != nil {
		return nil, chord.ErrorMapper(err)
	}

	succList := resp.GetSuccessors()
	nodes := make([]chord.VNode, 0, len(succList))

	for _, succ := range succList {
		node, err := NewRemoteNode(n.baseContext, n.baseLogger, n.chordClient, succ)
		if err != nil {
			n.logger.Error("error creating RemoteNote in GetSuccessors", zap.Object("peer", n.Identity()), zap.Error(err))
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (n *RemoteNode) GetPredecessor() (chord.VNode, error) {
	ctx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, n.identity), rpcTimeout)
	defer cancel()

	resp, err := n.chordClient.GetPredecessor(ctx, &protocol.GetPredecessorRequest{})
	if err != nil {
		return nil, chord.ErrorMapper(err)
	}

	if resp.GetPredecessor() == nil {
		return nil, nil
	}
	if resp.GetPredecessor().GetId() == n.ID() {
		return n, nil
	}

	pre, err := NewRemoteNode(n.baseContext, n.baseLogger, n.chordClient, resp.GetPredecessor())
	if err != nil {
		return nil, err
	}

	return pre, nil
}

func (n *RemoteNode) Put(ctx context.Context, key, value []byte) error {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.Put(reqCtx, &protocol.SimpleRequest{
		Key:   key,
		Value: value,
	})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) Get(ctx context.Context, key []byte) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	resp, err := n.chordClient.Get(reqCtx, &protocol.SimpleRequest{
		Key: key,
	})
	if err != nil {
		return nil, chord.ErrorMapper(err)
	}
	return resp.GetValue(), nil
}

func (n *RemoteNode) Delete(ctx context.Context, key []byte) error {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.Delete(reqCtx, &protocol.SimpleRequest{
		Key: key,
	})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) PrefixAppend(ctx context.Context, prefix []byte, child []byte) error {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.Append(reqCtx, &protocol.PrefixRequest{
		Prefix: prefix,
		Child:  child,
	})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) PrefixList(ctx context.Context, prefix []byte) ([][]byte, error) {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	resp, err := n.chordClient.List(reqCtx, &protocol.PrefixRequest{
		Prefix: prefix,
	})
	if err != nil {
		return nil, chord.ErrorMapper(err)
	}
	return resp.GetChildren(), nil
}

func (n *RemoteNode) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	resp, err := n.chordClient.Contains(reqCtx, &protocol.PrefixRequest{
		Prefix: prefix,
		Child:  child,
	})
	if err != nil {
		return false, chord.ErrorMapper(err)
	}
	return resp.GetExists(), nil
}

func (n *RemoteNode) PrefixRemove(ctx context.Context, prefix []byte, child []byte) error {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.Remove(reqCtx, &protocol.PrefixRequest{
		Prefix: prefix,
		Child:  child,
	})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) Acquire(ctx context.Context, lease []byte, ttl time.Duration) (uint64, error) {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	resp, err := n.chordClient.Acquire(reqCtx, &protocol.LeaseRequest{
		Lease: lease,
		Ttl:   durationpb.New(ttl),
	})
	if err != nil {
		return 0, chord.ErrorMapper(err)
	}
	return resp.GetToken(), nil
}

func (n *RemoteNode) Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	resp, err := n.chordClient.Renew(reqCtx, &protocol.LeaseRequest{
		Lease:     lease,
		Ttl:       durationpb.New(ttl),
		PrevToken: prevToken,
	})
	if err != nil {
		return 0, chord.ErrorMapper(err)
	}
	return resp.GetToken(), nil
}

func (n *RemoteNode) Release(ctx context.Context, lease []byte, token uint64) error {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.Release(reqCtx, &protocol.LeaseRequest{
		Lease:     lease,
		PrevToken: token,
	})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) Import(ctx context.Context, keys [][]byte, values []*protocol.KVTransfer) error {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(ctx, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.Import(reqCtx, &protocol.ImportRequest{
		Keys:   keys,
		Values: values,
	})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) RequestToJoin(joiner chord.VNode) (chord.VNode, []chord.VNode, error) {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, n.identity), rpcTimeout)
	defer cancel()

	resp, err := n.chordClient.RequestToJoin(reqCtx, &protocol.RequestToJoinRequest{
		Joiner: joiner.Identity(),
	})
	if err != nil {
		return nil, nil, chord.ErrorMapper(err)
	}

	pre := resp.GetPredecessor()
	succList := resp.GetSuccessors()

	pp, err := NewRemoteNode(n.baseContext, n.baseLogger, n.chordClient, pre)
	if err != nil {
		return nil, nil, err
	}

	nodes := make([]chord.VNode, 0, len(succList))
	for _, succ := range succList {
		node, err := NewRemoteNode(n.baseContext, n.baseLogger, n.chordClient, succ)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	return pp, nodes, nil
}

func (n *RemoteNode) FinishJoin(stablize bool, release bool) error {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.FinishJoin(reqCtx, &protocol.MembershipConclusionRequest{
		Stablize: stablize,
		Release:  release,
	})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) RequestToLeave(leaver chord.VNode) error {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.RequestToLeave(reqCtx, &protocol.RequestToLeaveRequest{
		Leaver: leaver.Identity(),
	})

	return chord.ErrorMapper(err)
}

func (n *RemoteNode) FinishLeave(stablize bool, release bool) error {
	reqCtx, cancel := context.WithTimeout(rpc.WithNode(n.baseContext, n.identity), rpcTimeout)
	defer cancel()

	_, err := n.chordClient.FinishLeave(reqCtx, &protocol.MembershipConclusionRequest{
		Stablize: stablize,
		Release:  release,
	})

	return chord.ErrorMapper(err)
}
