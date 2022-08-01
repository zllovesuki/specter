package chord

import "fmt"

var (
	ErrNodeGone         = fmt.Errorf("node is not part of the chord ring")
	ErrKVStaleOwnership = fmt.Errorf("processing node no longer has ownership over requested key")
)
