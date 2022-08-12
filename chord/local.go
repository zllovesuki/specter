package chord

import (
	"sync"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/atomic"
)

type LocalNode struct {
	successors     atomic.Pointer[[]chord.VNode]
	predecessorMu  sync.RWMutex
	surrogateMu    sync.RWMutex
	kv             chord.KVProvider
	predecessor    chord.VNode
	succListHash   *atomic.Uint64
	lastStabilized *atomic.Time
	surrogate      *protocol.Node
	stopCh         chan struct{}
	state          *nodeState
	fingers        []atomic.Pointer[chord.VNode]
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
	var emptyNode chord.VNode
	for i := range n.fingers {
		n.fingers[i].Store(&emptyNode)
	}

	return n
}
