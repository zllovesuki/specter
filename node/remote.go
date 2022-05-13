package node

import (
	"context"

	"specter/chord"
	"specter/overlay"
	"specter/spec/protocol"
)

type RemoteNode struct {
	parentCtx context.Context

	id  *protocol.Node
	rpc *overlay.RPC
	t   *overlay.Transport
}

var _ chord.VNode = &RemoteNode{}

func NewUnknownRemoteNode(ctx context.Context, t *overlay.Transport, addr string) (*RemoteNode, error) {
	n := &RemoteNode{
		parentCtx: ctx,
		t:         t,
	}
	r, err := t.DialRPC(ctx, addr, n.handshake)
	if err != nil {
		return nil, err
	}
	n.rpc = r
	return n, nil
}

func NewKnownRemoteNode(ctx context.Context, t *overlay.Transport, peer *protocol.Node) (*RemoteNode, error) {
	n := &RemoteNode{
		parentCtx: ctx,
		id:        peer,
		t:         t,
	}
	r, err := t.DialRPC(ctx, peer.GetAddress(), nil)
	if err != nil {
		return nil, err
	}
	n.rpc = r
	return n, nil
}

func (n *RemoteNode) handshake(r *overlay.RPC) error {
	rReq := newRR(protocol.RequestReply_IDENTITY)
	rResp, err := r.Call(rReq)
	if err != nil {
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
	return err
}

func (n *RemoteNode) Notify(predecessor chord.VNode) error {
	rReq := newRR(protocol.RequestReply_NOTIFY)
	rReq.NotifyRequest = &protocol.NotifyRequest{
		Predecessor: predecessor.Identity(),
	}
	_, err := n.rpc.Call(rReq)
	return err
}

func (n *RemoteNode) FindSuccessor(key uint64) (chord.VNode, error) {
	rReq := newRR(protocol.RequestReply_FIND_SUCCESSOR)
	rReq.FindSuccessorRequest = &protocol.FindSuccessorRequest{
		Key: key,
	}
	rResp, err := n.rpc.Call(rReq)
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

	succ, err := NewKnownRemoteNode(n.parentCtx, n.t, resp.GetSuccessor())
	if err != nil {
		return nil, err
	}

	return succ, nil
}

func (n *RemoteNode) GetPredecessor() (chord.VNode, error) {
	rReq := newRR(protocol.RequestReply_GET_PREDECESSOR)
	rReq.GetPredecessorRequest = &protocol.GetPredecessorRequest{}
	rResp, err := n.rpc.Call(rReq)
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

	pre, err := NewKnownRemoteNode(n.parentCtx, n.t, resp.GetPredecessor())
	if err != nil {
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
	return err
}
