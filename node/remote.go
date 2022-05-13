package node

import (
	"context"
	"specter/chord"
	"specter/overlay"
	"specter/spec/protocol"

	"github.com/lucas-clemente/quic-go"
)

type RemoteNode struct {
	parentCtx context.Context

	id *protocol.Node
	s  quic.Stream
	t  *overlay.Transport
}

var _ chord.VNode = &RemoteNode{}

func NewRemoteNode(ctx context.Context, t *overlay.Transport, node *protocol.Node) (*RemoteNode, error) {
	stream, err := t.Dial(ctx, node)
	if err != nil {
		return nil, err
	}
	n := &RemoteNode{
		parentCtx: ctx,
		s:         stream,
		t:         t,
	}
	if err := n.handshake(); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *RemoteNode) handshake() error {
	// exchange identity
	identity := &protocol.Node{}
	if err := overlay.RequestReply(n.s, n.t.Identifty(), identity); err != nil {
		return err
	}

	n.id = identity
	return nil
}

func (n *RemoteNode) ID() uint64 {
	return n.id.GetId()
}

func (n *RemoteNode) Identity() *protocol.Node {
	return n.id
}

func (n *RemoteNode) Ping() error {
	resp := &protocol.PingResponse{}
	if err := overlay.RequestReply(n.s, &protocol.PingRequest{}, resp); err != nil {
		return err
	}
	return nil
}

func (n *RemoteNode) Notify(predecessor chord.VNode) error {
	req := &protocol.NotifyRequest{
		Predecessor: predecessor.Identity(),
	}
	resp := &protocol.NotifyResponse{}
	if err := overlay.RequestReply(n.s, req, resp); err != nil {
		return err
	}
	return nil
}

func (n *RemoteNode) FindSuccessor(key uint64) (chord.VNode, error) {
	req := &protocol.FindSuccessorRequest{
		Key: key,
	}
	resp := &protocol.FindSuccessorResponse{}
	if err := overlay.RequestReply(n.s, req, resp); err != nil {
		return nil, err
	}

	if resp.Successor.GetId() == n.ID() {
		return n, nil
	}

	succ, err := NewRemoteNode(n.parentCtx, n.t, resp.Successor)
	if err != nil {
		return nil, err
	}

	return succ, nil
}

func (n *RemoteNode) GetPredecessor() (chord.VNode, error) {
	req := &protocol.GetPredecessorRequest{}
	resp := &protocol.GetPredecessorResponse{}
	if err := overlay.RequestReply(n.s, req, resp); err != nil {
		return nil, err
	}

	if resp.Predecessor.GetId() == n.ID() {
		return n, nil
	}

	pre, err := NewRemoteNode(n.parentCtx, n.t, resp.Predecessor)
	if err != nil {
		return nil, err
	}

	return pre, nil
}

func (n *RemoteNode) Put(key, value []byte) error {
	// TODO: RPC
	return nil
}

func (n *RemoteNode) Get(key []byte) ([]byte, error) {
	// TODO: RPC
	return nil, nil
}

func (n *RemoteNode) Delete(key []byte) error {
	// TODO: RPC
	return nil
}
