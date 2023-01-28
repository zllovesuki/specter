package chord

import (
	"sync"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/puzpuzpuz/xsync/v2"
	"go.uber.org/atomic"
)

type LocalNode struct {
	predecessorMu  *xsync.RBMutex                          // simple mutex surrounding operations on predecessor
	predecessor    chord.VNode                             // nord's immediate predecessor
	_              [0]__                                   //
	surrogateMu    *xsync.RBMutex                          // advanced mutex surrounding KV requests during Join/Leave
	surrogate      *protocol.Node                          // node's previous predecessor, used to guard against outdated KV requests
	_              [0]__                                   //
	successorsMu   *xsync.RBMutex                          // advanced mutex surrounding successors during Join/Leave
	successors     []chord.VNode                           // node's extended list of successors
	succListHash   *atomic.Uint64                          // simple hash on the successors to determine if they have changed
	_              [0]__                                   //
	kv             chord.KVProvider                        // KV backing implementation
	lastStabilized *atomic.Time                            // informational only
	_              [0]__                                   //
	stopWg         sync.WaitGroup                          // used to wait for task goroutines to be stopped
	stopCh         chan struct{}                           // used to signal task goroutines to stop
	_              [0]__                                   //
	state          *nodeState                              // a replacement of LockQueue from the paper
	fingers        [chord.MaxFingerEntries + 1]fingerEntry // finger table to provide log(N) optimization
	NodeConfig
}

type __ any

var _ chord.VNode = (*LocalNode)(nil)

type fingerEntry struct {
	*xsync.RBMutex
	node chord.VNode
}

func (f *fingerEntry) computeUpdate(fn func(entry *fingerEntry)) {
	f.Lock()
	defer f.Unlock()

	fn(f)
}

func (f *fingerEntry) computeView(fn func(node chord.VNode)) {
	t := f.RLock()
	defer f.RUnlock(t)

	fn(f.node)
}

func NewLocalNode(conf NodeConfig) *LocalNode {
	if err := conf.Validate(); err != nil {
		panic(err)
	}
	n := &LocalNode{
		NodeConfig:     conf,
		predecessorMu:  xsync.NewRBMutex(),
		surrogateMu:    xsync.NewRBMutex(),
		successorsMu:   xsync.NewRBMutex(),
		state:          NewNodeState(chord.Inactive),
		succListHash:   atomic.NewUint64(conf.Identity.GetId()),
		kv:             conf.KVProvider,
		lastStabilized: atomic.NewTime(time.Time{}),
		stopCh:         make(chan struct{}),
	}
	for k := 1; k <= chord.MaxFingerEntries; k++ {
		n.fingers[k].RBMutex = xsync.NewRBMutex()
	}
	n.stopWg.Add(3)

	return n
}
