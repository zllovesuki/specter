package chord

import (
	"sync"
	"sync/atomic"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	atom "go.uber.org/atomic"
)

type LocalNode struct {
	successors     atomic.Pointer[[]chord.VNode]
	predecessorMu  sync.RWMutex
	surrogateMu    sync.RWMutex
	kv             chord.KVProvider
	predecessor    chord.VNode
	succListHash   *atom.Uint64
	lastStabilized *atom.Time
	surrogate      *protocol.Node
	stopCh         chan struct{}
	fingers        []atomic.Pointer[chord.VNode]
	NodeConfig
	state chord.State
}

var _ chord.VNode = (*LocalNode)(nil)

func NewLocalNode(conf NodeConfig) *LocalNode {
	if err := conf.Validate(); err != nil {
		panic(err)
	}
	n := &LocalNode{
		NodeConfig:     conf,
		state:          chord.Inactive,
		succListHash:   atom.NewUint64(conf.Identity.GetId()),
		kv:             conf.KVProvider,
		fingers:        make([]atomic.Pointer[chord.VNode], chord.MaxFingerEntries+1),
		lastStabilized: atom.NewTime(time.Time{}),
		stopCh:         make(chan struct{}),
	}
	var emptyNode chord.VNode
	for i := range n.fingers {
		n.fingers[i].Store(&emptyNode)
	}

	return n
}
