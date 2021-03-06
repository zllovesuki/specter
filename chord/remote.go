package chord

import (
	"context"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"

	"go.uber.org/zap"
)

const (
	rpcTimeout  = time.Second * 5
	pingTimeout = time.Second
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
		// n.logger.Error("remote Identity RPC", zap.String("node", n.Identity().String()), zap.Error(err))
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

// this is needed because RPC call squash type information, so in call site with signature
// if err == ErrABC will fail (but err.Error() == ErrABC.Error() will work).
func errorMapper(resp *protocol.RPC_Response, err error) (*protocol.RPC_Response, error) {
	if err == nil {
		return resp, err
	}
	var parsedErr error
	switch err.Error() {
	case chord.ErrKVStaleOwnership.Error():
		parsedErr = chord.ErrKVStaleOwnership
	case chord.ErrNodeGone.Error():
		parsedErr = chord.ErrNodeGone
	default:
		// passthrough
		parsedErr = err
	}
	return resp, parsedErr
}

func (n *RemoteNode) Ping() error {
	ctx, cancel := context.WithTimeout(n.parentCtx, pingTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_PING)
	rReq.PingRequest = &protocol.PingRequest{}
	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
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
	if err != nil {
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
		n.logger.Error("remote FindSuccessor RPC", zap.String("peer", n.Identity().String()), zap.Uint64("key", key), zap.Error(err))
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
		n.logger.Error("creating new RemoteNode", zap.String("peer", resp.GetSuccessor().String()), zap.Error(err))
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
		n.logger.Error("remote GetSuccessors RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
		return nil, err
	}

	resp := rResp.GetGetSuccessorsResponse()
	succList := resp.GetSuccessors()

	nodes := make([]chord.VNode, 0, len(succList))
	for _, succ := range succList {
		node, err := createRPC(n.parentCtx, n.transport, n.logger, succ)
		if err != nil {
			n.logger.Error("create RemoteNote in GetSuccessors", zap.String("peer", n.Identity().String()), zap.Error(err))
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
		n.logger.Error("remote GetPredecessor RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
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
		n.logger.Error("creating new RemoteNode", zap.String("peer", resp.GetPredecessor().String()), zap.Error(err))
		return nil, err
	}

	return pre, nil
}

func (n *RemoteNode) Put(key, value []byte) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:    protocol.KVOperation_PUT,
		Key:   key,
		Value: value,
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		n.logger.Error("remote KV Put RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) Get(key []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_GET,
		Key: key,
	}

	rResp, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		n.logger.Error("remote KV Get RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
		return nil, err
	}
	return rResp.GetKvResponse().GetValue(), nil
}

func (n *RemoteNode) Delete(key []byte) error {
	ctx, cancel := context.WithTimeout(n.parentCtx, rpcTimeout)
	defer cancel()

	rReq := newReq(protocol.RPC_KV)
	rReq.KvRequest = &protocol.KVRequest{
		Op:  protocol.KVOperation_DELETE,
		Key: key,
	}

	_, err := errorMapper(n.rpc.Call(ctx, rReq))
	if err != nil {
		n.logger.Error("remote KV Delete RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
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
	if err != nil {
		n.logger.Error("remote KV DirectPuts RPC", zap.String("peer", n.Identity().String()), zap.Error(err))
	}
	return err
}

func (n *RemoteNode) Export(keys [][]byte) []*protocol.KVTransfer {
	panic("Export is not a valid RPC method")
}

func (n *RemoteNode) RangeKeys(low, high uint64) [][]byte {
	panic("RangeKeys is not a valid RPC method")
}

func (n *RemoteNode) RemoveKeys(keys [][]byte) {
	panic("RemoveKeys is not a valid RPC method")
}

func (n *RemoteNode) Stop() {
	n.rpc.Close()
}
