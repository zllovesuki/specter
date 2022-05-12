package chord

type RemoteNode struct {
	id uint64
}

var _ VNode = &RemoteNode{}

func (n *RemoteNode) ID() uint64 {
	return n.id
}

func (n *RemoteNode) Ping() error {
	// TODO: RPC
	return nil
}

func (n *RemoteNode) Notify(predecessor VNode) error {
	// TODO: RPC
	return nil
}

func (n *RemoteNode) FindSuccessor(key uint64) (VNode, error) {
	// TODO: RPC
	return nil, nil
}

func (n *RemoteNode) GetPredecessor() (VNode, error) {
	// TODO: RPC
	return nil, nil
}

func (n *RemoteNode) IsBetween(low, high VNode) bool {
	// TODO: RPC
	return true
}

func (n *RemoteNode) CheckPredecessor() error {
	panic("CheckPredecessor not implemented in RemoteNode")
}
