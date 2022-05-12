package chord

type RemoteNode struct {
	id uint64
}

var _ VNode = &RemoteNode{}

func (n *RemoteNode) ID() uint64 {
	return n.id
}

func (n *RemoteNode) Ping() error {
	return nil
}

func (n *RemoteNode) Notify(predecessor VNode) error {
	return nil
}

func (n *RemoteNode) FindSuccessor(key uint64) (VNode, error) {
	return nil, nil
}

func (n *RemoteNode) GetPredecessor() (VNode, error) {
	return nil, nil
}

func (n *RemoteNode) CheckPredecessor() error {
	return nil
}

func (n *RemoteNode) IsBetween(low, high VNode) bool {
	return true
}
