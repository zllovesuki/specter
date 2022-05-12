package chord

type VNode interface {
	ID() uint64

	Ping() error

	Notify(predecessor VNode) error

	FindSuccessor(key uint64) (VNode, error)
	GetPredecessor() (VNode, error)
	CheckPredecessor() error

	IsBetween(low, high VNode) bool
}
