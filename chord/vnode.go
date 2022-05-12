package chord

type VNode interface {
	KV

	ID() uint64

	Ping() error

	Notify(predecessor VNode) error

	FindSuccessor(key uint64) (VNode, error)
	GetPredecessor() (VNode, error)
}
