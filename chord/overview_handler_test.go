package chord

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"

	"github.com/stretchr/testify/require"
)

// The nil embedded provider panics if the overview attempts any storage access.
type unavailableOverviewStore struct{ chord.KVProvider }

func TestOverviewUsesLocalObservationsWithoutStorageOrRPC(t *testing.T) {
	as := require.New(t)
	conf := devConfig(t, as)
	// Unexpected calls fail the test, including key enumeration or export.
	conf.KVProvider = &unavailableOverviewStore{}
	conf.Identity.Address = "local.example:443"
	root := NewLocalNode(conf)
	root.state.Set(chord.Active)
	root.chordRate.IncrementBy(4)
	root.kvRate.IncrementBy(5)
	root.rpcErrorCount.Store(2)
	root.kvStaleCount.Store(3)
	stabilized := time.Now().UTC().Add(-time.Minute)
	root.lastStabilized.Store(stabilized)

	otherConf := devConfig(t, as)
	otherConf.KVProvider = &unavailableOverviewStore{}
	other := NewLocalNode(otherConf)
	other.state.Set(chord.Joining)
	root.predecessor = other
	root.successors = []chord.VNode{nil, other}

	w := httptest.NewRecorder()
	before := time.Now().UTC()
	OverviewHandler(root, []*LocalNode{root, other}, "sqlite").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_internal/overview.json", nil))
	as.Equal(http.StatusOK, w.Code)
	as.Equal("no-store", w.Header().Get("Cache-Control"))
	as.Contains(w.Header().Get("Content-Type"), "application/json")
	var info localOverview
	as.NoError(json.Unmarshal(w.Body.Bytes(), &info))
	as.WithinRange(info.ObservedAt, before, time.Now().UTC())
	as.Equal("local.example:443", info.Identity.Address)
	as.Equal("sqlite", info.Provider)
	as.Len(info.VNodes, 2)
	vnode := info.VNodes[0]
	as.Equal("Active", vnode.State)
	as.True(vnode.Root)
	as.Equal(stabilized, *vnode.LastStabilized)
	as.EqualValues(4, vnode.ChordRequests)
	as.EqualValues(5, vnode.KVRequests)
	as.EqualValues(2, vnode.RPCErrors)
	as.EqualValues(3, vnode.KVStale)
	as.True(vnode.PredecessorAvailable)
	as.True(vnode.SuccessorsAvailable)
	as.NotNil(vnode.Predecessor)
	as.Len(vnode.Successors, 1)
	as.Equal(vnode.Predecessor.ID, vnode.Successors[0].ID)
	as.Equal("Joining", info.VNodes[1].State)
	as.False(info.VNodes[1].Root)
	as.Nil(info.VNodes[1].LastStabilized)
	as.True(info.VNodes[1].PredecessorAvailable)
	as.Nil(info.VNodes[1].Predecessor)
	as.NotNil(info.VNodes[1].Successors, "empty successors must serialize as an array")
	as.Empty(info.VNodes[1].Successors)
}

func TestOverviewPreservesCountersWhileMembershipIsLocked(t *testing.T) {
	as := require.New(t)
	root := NewLocalNode(devConfig(t, as))
	root.state.Set(chord.Transferring)
	root.rpcErrorCount.Store(9)
	root.predecessorMu.Lock()
	defer root.predecessorMu.Unlock()
	root.successorsMu.Lock()
	defer root.successorsMu.Unlock()

	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		OverviewHandler(root, []*LocalNode{root}, "aof").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_internal/overview.json", nil))
		result <- w
	}()
	select {
	case w := <-result:
		as.Equal(http.StatusOK, w.Code)
		var info localOverview
		as.NoError(json.Unmarshal(w.Body.Bytes(), &info))
		as.Equal("Transferring", info.VNodes[0].State)
		as.EqualValues(9, info.VNodes[0].RPCErrors)
		as.False(info.VNodes[0].PredecessorAvailable)
		as.False(info.VNodes[0].SuccessorsAvailable)
	case <-time.After(time.Second):
		t.Fatal("overview blocked on a membership lock")
	}
}
