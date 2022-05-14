package node

import (
	"context"

	"specter/overlay"
	"specter/rpc"
	"specter/spec/chord"
	"specter/spec/protocol"

	"go.uber.org/zap"
)

type RemoteNode struct {
	parentCtx context.Context

	logger *zap.Logger

	id  *protocol.Node
	rpc *rpc.RPC
	t   *overlay.Transport
}

var _ chord.VNode = (*RemoteNode)(nil)

func NewRemoteNode(ctx context.Context, t *overlay.Transport, logger *zap.Logger, peer *protocol.Node) (*RemoteNode, error) {
	n := &RemoteNode{
		parentCtx: ctx,
		id:        peer,
		t:         t,
		logger:    logger,
	}
	var hs overlay.RPCHandshakeFunc
	if peer.GetUnknown() {
		hs = n.handshake
	}
	r, err := t.DialRPC(ctx, peer, protocol.Stream_PEER, hs)
	if err != nil {
		return nil, err
	}
	n.rpc = r
	return n, nil
}

func (n *RemoteNode) handshake(r *rpc.RPC) error {
	rReq := newRR(protocol.RequestReply_IDENTITY)
	rResp, err := r.Call(rReq)
	if err != nil {
		n.logger.Error("remote Identity RPC", zap.String("node", n.Identity().String()), zap.Error(err))
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

func newRR(t protocol.RequestReply_Kind) *protocol.RequestReply {
	return &protocol.RequestReply{
		Kind: t,
		Type: protocol.RequestReply_REQUEST,
	}
}

func (n *RemoteNode) Ping() error {
	rReq := newRR(protocol.RequestReply_PING)
	rReq.PingRequest = &protocol.PingRequest{}
	_, err := n.rpc.Call(rReq)
	if err != nil {
		n.logger.Error("remote Ping RPC", zap.String("node", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) Notify(predecessor chord.VNode) error {
	rReq := newRR(protocol.RequestReply_NOTIFY)
	rReq.NotifyRequest = &protocol.NotifyRequest{
		Predecessor: predecessor.Identity(),
	}
	_, err := n.rpc.Call(rReq)
	if err != nil {
		n.logger.Error("remote Notify RPC", zap.String("node", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) FindSuccessor(key uint64) (chord.VNode, error) {
	rReq := newRR(protocol.RequestReply_FIND_SUCCESSOR)
	rReq.FindSuccessorRequest = &protocol.FindSuccessorRequest{
		Key: key,
	}
	rResp, err := n.rpc.Call(rReq)
	if err != nil {
		n.logger.Error("remote FindSuccessor RPC", zap.String("node", n.Identity().String()), zap.Uint64("key", key), zap.Error(err))
		return nil, err
	}

	resp := rResp.GetFindSuccessorResponse()
	if resp.GetSuccessor() == nil {
		return nil, nil
	}

	if resp.GetSuccessor().GetId() == n.ID() {
		return n, nil
	}

	succ, err := NewRemoteNode(n.parentCtx, n.t, n.logger, resp.GetSuccessor())
	if err != nil {
		n.logger.Error("creating new RemoteNode", zap.String("node", resp.GetSuccessor().String()), zap.Error(err))
		return nil, err
	}

	return succ, nil
}

func (n *RemoteNode) GetSuccessors() ([]chord.VNode, error) {
	rReq := newRR(protocol.RequestReply_GET_SUCCESSORS)
	rReq.GetSuccessorsRequest = &protocol.GetSuccessorsRequest{}
	rResp, err := n.rpc.Call(rReq)
	if err != nil {
		n.logger.Error("remote GetSuccessors RPC", zap.String("node", n.Identity().String()), zap.Error(err))
		return nil, err
	}

	resp := rResp.GetGetSuccessorsResponse()
	succList := resp.GetSuccessors()

	nodes := make([]chord.VNode, 0, len(succList))
	for _, node := range succList {
		n, err := NewRemoteNode(n.parentCtx, n.t, n.logger, node)
		if err != nil {
			continue
		}
		nodes = append(nodes, n)
	}

	return nodes, nil
}

func (n *RemoteNode) GetPredecessor() (chord.VNode, error) {
	rReq := newRR(protocol.RequestReply_GET_PREDECESSOR)
	rReq.GetPredecessorRequest = &protocol.GetPredecessorRequest{}
	rResp, err := n.rpc.Call(rReq)
	if err != nil {
		n.logger.Error("remote GetPredecessor RPC", zap.String("node", n.Identity().String()), zap.Error(err))
		return nil, err
	}

	resp := rResp.GetGetPredecessorResponse()
	if resp.GetPredecessor() == nil {
		return nil, nil
	}
	if resp.GetPredecessor().GetId() == n.ID() {
		return n, nil
	}

	pre, err := NewRemoteNode(n.parentCtx, n.t, n.logger, resp.GetPredecessor())
	if err != nil {
		n.logger.Error("creating new RemoteNode", zap.String("node", resp.GetPredecessor().String()), zap.Error(err))
		return nil, err
	}

	return pre, nil
}

func (n *RemoteNode) Put(key, value []byte) error {
	rReq := newRR(protocol.RequestReply_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:    protocol.KVOperation_PUT,
		Key:   key,
		Value: value,
	}
	_, err := n.rpc.Call(rReq)
	if err != nil {
		n.logger.Error("remote KV Put RPC", zap.String("node", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) Get(key []byte) ([]byte, error) {
	rReq := newRR(protocol.RequestReply_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_GET,
		Key: key,
	}
	rResp, err := n.rpc.Call(rReq)
	if err != nil {
		n.logger.Error("remote KV Get RPC", zap.String("node", n.Identity().String()), zap.Error(err))
		return nil, err
	}
	return rResp.GetKvResponse().GetValue(), nil
}

func (n *RemoteNode) Delete(key []byte) error {
	rReq := newRR(protocol.RequestReply_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_DELETE,
		Key: key,
	}
	_, err := n.rpc.Call(rReq)
	if err != nil {
		n.logger.Error("remote KV Delete RPC", zap.String("node", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) Stop() {
	n.rpc.Close()
}
