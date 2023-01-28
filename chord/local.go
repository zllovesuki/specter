package chord

import (
	"sync"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/atomic"
)

type LocalNode struct {
	predecessorMu  sync.RWMutex                            // simple mutex surrounding operations on predecessor
	predecessor    chord.VNode                             // nord's immediate predecessor
	surrogateMu    sync.RWMutex                            // advanced mutex surrounding KV requests during Join/Leave
	surrogate      *protocol.Node                          // node's previous predecessor, used to guard against outdated KV requests
	successorsMu   sync.Mutex                              // advanced mutex surrounding successors during Join/Leave
	successors     atomic.Pointer[[]chord.VNode]           // node's immediate successors
	succListHash   *atomic.Uint64                          // simple hash on the successors to determine if they have changed
	kv             chord.KVProvider                        // KV backing implementation
	lastStabilized *atomic.Time                            // informational only
	stopWg         sync.WaitGroup                          // used to wait for task goroutines to be stopped
	stopCh         chan struct{}                           // used to signal task goroutines to stop
	state          *nodeState                              // a replacement of LockQueue from the paper
	fingers        [chord.MaxFingerEntries + 1]fingerEntry // finger table to provide log(N) optimization
	NodeConfig
}

type fingerEntry struct {
	mu    sync.RWMutex
	node  chord.VNode
	start uint64
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
		lastStabilized: atomic.NewTime(time.Time{}),
		stopCh:         make(chan struct{}),
	}
	for k := 1; k <= chord.MaxFingerEntries; k++ {
		entry := &n.fingers[k]
		entry.node = nil
		entry.start = chord.ModuloSum(n.ID(), 1<<(k-1))
	}
	n.stopWg.Add(3)

	return n
}
