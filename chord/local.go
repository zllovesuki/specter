package chord

import (
	"sync"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/atomic"
)

type LocalNode struct {
	predecessorMu  sync.RWMutex                  // simple mutex surrounding operations on predecessor
	predecessor    chord.VNode                   // nord's immediate predecessor
	surrogateMu    sync.RWMutex                  // advanced mutex surrounding KV requests during Join/Leave
	surrogate      *protocol.Node                // node's previous predecessor, used to guard against outdated KV requests
	successorsMu   sync.Mutex                    // advanced mutex surrounding successors during Join/Leave
	successors     atomic.Pointer[[]chord.VNode] // node's immediate successors
	succListHash   *atomic.Uint64                // simple hash on the successors to determine if they have changed
	kv             chord.KVProvider              // KV backing implementation
	lastStabilized *atomic.Time                  // informational only
	stopCh         chan struct{}                 // used to signal task goroutines to stop
	state          *nodeState                    // a replacement of LockQueue from the paper
	fingers        []atomic.Pointer[chord.VNode] // finger table to provide log(N) optimization
	NodeConfig
}

var _ chord.VNode = (*LocalNode)(nil)

func NewLocalNode(conf NodeConfig) *LocalNode {
	if err := conf.Validate(); err != nil {
		panic(err)
	}
	n := &LocalNode{
		NodeConfig:     conf,
		state:          NewNodeState(chord.Inactive),
		succListHash:   atomic.NewUint64(conf.Identity.GetId()),
		kv:             conf.KVProvider,
		fingers:        make([]atomic.Pointer[chord.VNode], chord.MaxFingerEntries+1),
		lastStabilized: atomic.NewTime(time.Time{}),
		stopCh:         make(chan struct{}),
	}
	for i := range n.fingers {
		var emptyNode chord.VNode
		n.fingers[i].Store(&emptyNode)
	}

	return n
}
