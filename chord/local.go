package chord

import (
	"net/http"
	"sync"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/util/acceptor"
	"go.miragespace.co/specter/util/ratecounter"

	"github.com/TheZeroSlave/zapsentry"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type LocalNode struct {
	logger         *zap.Logger
	predecessorMu  sync.RWMutex                            // simple mutex surrounding operations on predecessor
	predecessor    chord.VNode                             // nord's immediate predecessor
	surrogateMu    sync.RWMutex                            // advanced mutex surrounding KV requests during Join/Leave
	surrogate      chord.VNode                             // node's previous predecessor, used to guard against outdated KV requests
	successorsMu   sync.RWMutex                            // advanced mutex surrounding successors during Join/Leave
	successors     []chord.VNode                           // node's extended list of successors
	succListHash   *atomic.Uint64                          // simple hash on the successors to determine if they have changed
	kv             chord.KVProvider                        // KV backing implementation
	chordRate      *ratecounter.Rate                       // track how chatty incoming chord rpc is
	kvRate         *ratecounter.Rate                       // track how chatty incoming kv rpc is
	kvStaleCount   *atomic.Uint64                          // track the number of kv stale ownership
	rpcErrorCount  *atomic.Uint64                          // track the number of non-retryable rpc request errors
	lastStabilized *atomic.Time                            // last stablized time
	stopWg         sync.WaitGroup                          // used to wait for task goroutines to be stopped
	stopCh         chan struct{}                           // used to signal task goroutines to stop
	state          *nodeState                              // a replacement of LockQueue from the paper
	fingers        [chord.MaxFingerEntries + 1]fingerEntry // finger table to provide log(N) optimization
	rpcAcceptor    *acceptor.HTTP2Acceptor                 // listener for handling incoming rpc request
	rpcHandler     http.Handler
	rpcHandlerOnce sync.Once
	NodeConfig
}

var _ chord.VNode = (*LocalNode)(nil)

type fingerEntry struct {
	node chord.VNode
	sync.RWMutex
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
		logger:         conf.BaseLogger.With(zapsentry.NewScope()).With(zap.String("component", "localNode"), zap.Uint64("node", conf.Identity.GetId())),
		state:          newNodeState(chord.Inactive),
		succListHash:   atomic.NewUint64(conf.Identity.GetId()),
		kv:             conf.KVProvider,
		lastStabilized: atomic.NewTime(time.Time{}),
		stopCh:         make(chan struct{}),
		rpcAcceptor:    acceptor.NewH2Acceptor(nil),
		chordRate:      ratecounter.New(time.Second, time.Second*10),
		kvRate:         ratecounter.New(time.Second, time.Second*10),
		kvStaleCount:   atomic.NewUint64(0),
		rpcErrorCount:  atomic.NewUint64(0),
	}

	return n
}
