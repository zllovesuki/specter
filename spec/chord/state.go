//go:generate stringer -type=State
package chord

import "sync/atomic"

type State uint64

const (
	// Node not running, default state
	Inactive State = iota
	// In the progress of joining the network
	Joining
	// Ready to handle lookup and KV requests
	Active
	// Handling join request from another node
	Transferring
	// Leaving and transfering keys to successor
	Leaving
	// No longer an active node
	Left
)

func (s *State) Transition(expected State, new State) bool {
	return atomic.CompareAndSwapUint64((*uint64)(s), uint64(expected), uint64(new))
}

func (s *State) Get() State {
	return State(atomic.LoadUint64((*uint64)(s)))
}

func (s *State) Set(val State) {
	atomic.StoreUint64((*uint64)(s), uint64(val))
}
