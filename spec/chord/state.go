//go:generate stringer -type=State
package chord

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
	// Leaving and transferring keys to successor
	Leaving
	// No longer an active node
	Left
)
