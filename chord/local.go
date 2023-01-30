package chord

import (
	"sync"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/util/acceptor"
	"kon.nect.sh/specter/util/ratecounter"

	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type LocalNode struct {
	logger         *zap.Logger
	predecessorMu  sync.RWMutex                            // simple mutex surrounding operations on predecessor
	predecessor    chord.VNode                             // nord's immediate predecessor
	_              [0]any                                  //
	surrogateMu    sync.RWMutex                            // advanced mutex surrounding KV requests during Join/Leave
	surrogate      *protocol.Node                          // node's previous predecessor, used to guard against outdated KV requests
	_              [0]any                                  //
	successorsMu   sync.RWMutex                            // advanced mutex surrounding successors during Join/Leave
	successors     []chord.VNode                           // node's extended list of successors
	succListHash   *atomic.Uint64                          // simple hash on the successors to determine if they have changed
	_              [0]any                                  //
	kv             chord.KVProvider                        // KV backing implementation
	_              [0]any                                  //
	chordRate      *ratecounter.Rate                       // track how chatty chord rpc is
	kvRate         *ratecounter.Rate                       // track how chatty kv rpc is
	lastStabilized *atomic.Time                            // last stablized time
	_              [0]any                                  //
	stopWg         sync.WaitGroup                          // used to wait for task goroutines to be stopped
	stopCh         chan struct{}                           // used to signal task goroutines to stop
	_              [0]any                                  //
	state          *nodeState                              // a replacement of LockQueue from the paper
	fingers        [chord.MaxFingerEntries + 1]fingerEntry // finger table to provide log(N) optimization
	rpcAcceptor    *acceptor.HTTP2Acceptor                 // listener for handling incoming rpc request
	NodeConfig
}

var _ chord.VNode = (*LocalNode)(nil)

type fingerEntry struct {
	sync.RWMutex
	node chord.VNode
}

func (f *fingerEntry) computeUpdate(fn func(entry *fingerEntry)) {
	f.Lock()
	defer f.Unlock()

	fn(f)
}

func (f *fingerEntry) computeView(fn func(node chord.VNode)) {
	f.RLock()
	defer f.RUnlock()

	fn(f.node)
}

func NewLocalNode(conf NodeConfig) *LocalNode {
	if err := conf.Validate(); err != nil {
		panic(err)
	}
	n := &LocalNode{
		NodeConfig:     conf,
		logger:         conf.BaseLogger.With(zap.String("component", "localNode"), zap.Uint64("node", conf.Identity.GetId())),
		state:          NewNodeState(chord.Inactive),
		succListHash:   atomic.NewUint64(conf.Identity.GetId()),
		kv:             conf.KVProvider,
		lastStabilized: atomic.NewTime(time.Time{}),
		stopCh:         make(chan struct{}),
		rpcAcceptor:    acceptor.NewH2Acceptor(nil),
		chordRate:      ratecounter.New(time.Second, time.Second*10),
		kvRate:         ratecounter.New(time.Second, time.Second*10),
	}
	n.stopWg.Add(3)

	return n
}
