package chord

import "fmt"

var (
	ErrKVStaleOwnership = fmt.Errorf("processing node no longer has ownership over requested key")
)
