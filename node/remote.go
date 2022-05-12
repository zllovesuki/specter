package node

import "specter/chord"

type RemoteNode struct {
	id uint64
}

var _ chord.VNode = &RemoteNode{}

func (n *RemoteNode) Dial() error {
	// TODO: handshake and maintain connection via manager
	return nil
}

func (n *RemoteNode) ID() uint64 {
	return n.id
}

func (n *RemoteNode) Ping() error {
	// TODO: RPC
	return nil
}

func (n *RemoteNode) Notify(predecessor chord.VNode) error {
	// TODO: RPC
	return nil
}

func (n *RemoteNode) FindSuccessor(key uint64) (chord.VNode, error) {
	// TODO: RPC
	return nil, nil
}

func (n *RemoteNode) GetPredecessor() (chord.VNode, error) {
	// TODO: RPC
	return nil, nil
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
