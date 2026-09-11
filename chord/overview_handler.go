package chord

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"go.miragespace.co/specter/spec/protocol"
)

type overviewIdentity struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

type overviewVNode struct {
	Identity             overviewIdentity   `json:"identity"`
	Root                 bool               `json:"root"`
	State                string             `json:"state"`
	LastStabilized       *time.Time         `json:"last_stabilized"`
	ChordRequests        uint64             `json:"chord_requests"`
	KVRequests           uint64             `json:"kv_requests"`
	RPCErrors            uint64             `json:"rpc_errors"`
	KVStale              uint64             `json:"kv_stale"`
	PredecessorAvailable bool               `json:"predecessor_available"`
	Predecessor          *overviewIdentity  `json:"predecessor"`
	SuccessorsAvailable  bool               `json:"successors_available"`
	Successors           []overviewIdentity `json:"successors"`
}

type localOverview struct {
	ObservedAt time.Time        `json:"observed_at"`
	Identity   overviewIdentity `json:"identity"`
	Provider   string           `json:"provider"`
	VNodes     []overviewVNode  `json:"vnodes"`
}

func overviewNodeIdentity(node *protocol.Node) overviewIdentity {
	return overviewIdentity{
		ID:      strconv.FormatUint(node.GetId(), 10),
		Address: node.GetAddress(),
	}
}

// snapshotOverview samples local counters and membership references independently.
// It never consults storage or calls peers. Membership changes can hold locks
// across network requests, so a busy reference is reported as unavailable.
func snapshotOverview(rootNode *LocalNode, virtualNodes []*LocalNode, provider string) localOverview {
	info := localOverview{
		ObservedAt: time.Now().UTC(),
		Identity:   overviewNodeIdentity(rootNode.Identity()),
		Provider:   provider,
		VNodes:     make([]overviewVNode, 0, len(virtualNodes)),
	}
	for _, node := range virtualNodes {
		vnode := overviewVNode{
			Identity:      overviewNodeIdentity(node.Identity()),
			Root:          node == rootNode,
			State:         node.state.Get().String(),
			ChordRequests: node.chordRate.Total(),
			KVRequests:    node.kvRate.Total(),
			RPCErrors:     node.rpcErrorCount.Load(),
			KVStale:       node.kvStaleCount.Load(),
			Successors:    make([]overviewIdentity, 0),
		}
		if stabilized := node.lastStabilized.Load(); !stabilized.IsZero() {
			vnode.LastStabilized = &stabilized
		}
		if node.predecessorMu.TryRLock() {
			vnode.PredecessorAvailable = true
			if node.predecessor != nil {
				predecessor := overviewNodeIdentity(node.predecessor.Identity())
				vnode.Predecessor = &predecessor
			}
			node.predecessorMu.RUnlock()
		}
		if node.successorsMu.TryRLock() {
			vnode.SuccessorsAvailable = true
			for _, successor := range node.successors {
				if successor != nil {
					vnode.Successors = append(vnode.Successors, overviewNodeIdentity(successor.Identity()))
				}
			}
			node.successorsMu.RUnlock()
		}
		info.VNodes = append(info.VNodes, vnode)
	}
	return info
}

// OverviewHandler serves a bounded local JSON overview. The composition layer supplies
// the effective provider name; the diagnostic snapshot does not identify storage
// implementations or inspect their data directories.
//
// Mount this behind the same authentication as the other operator handlers at
// /_internal/overview.json.
func OverviewHandler(rootNode *LocalNode, virtualNodes []*LocalNode, provider string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := snapshotOverview(rootNode, virtualNodes, provider)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(info)
	})
}
