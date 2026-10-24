package chord

import (
	"errors"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
)

func TestFindSuccessorWhenFingersLag(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  [3]uint64
		key  uint64
	}{
		{name: "ordinary", ids: [3]uint64{10, 20, 30}, key: 25},
		{name: "wraparound", ids: [3]uint64{chord.MaxIdentitifer - 20, chord.MaxIdentitifer - 10, 10}, key: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			as := require.New(t)
			nodes := make([]*LocalNode, len(tc.ids))
			for i, id := range tc.ids {
				conf := devConfig(t, as)
				conf.Identity.Id = id
				nodes[i] = NewLocalNode(conf)
				nodes[i].state.Set(chord.Active)
			}
			for i, node := range nodes {
				node.predecessor = nodes[(i+len(nodes)-1)%len(nodes)]
				node.successors = []chord.VNode{nodes[(i+1)%len(nodes)]}
			}

			// The successor reflects a join, while fingers still point past the
			// requested key. Keep maintenance stopped so the gap cannot repair itself.
			for k := 1; k <= chord.MaxFingerEntries; k++ {
				nodes[0].fingers[k].node = nodes[2]
			}
			successor, err := nodes[0].FindSuccessor(tc.key)
			as.NoError(err)
			as.Same(nodes[2], successor)
		})
	}
}

func TestStabilizeRecordsSuccessfulExchange(t *testing.T) {
	failed := errors.New("peer unavailable")
	for _, tc := range []struct {
		name          string
		state         chord.State
		failedHead    bool
		probeErr      error
		notifyErr     error
		wantErr       error
		wantTimestamp bool
	}{
		{name: "unchanged successors", state: chord.Active, wantTimestamp: true},
		{name: "failed head with live successor", state: chord.Active, failedHead: true, wantTimestamp: true},
		{name: "all probes failed", state: chord.Active, probeErr: failed, wantErr: chord.ErrNodeNoSuccessor},
		{name: "notification failed after refresh", state: chord.Active, failedHead: true, notifyErr: failed, wantErr: failed},
		{name: "leaving skips notification", state: chord.Leaving},
	} {
		t.Run(tc.name, func(t *testing.T) {
			as := require.New(t)
			conf := devConfig(t, as)
			conf.Identity.Id = 10
			node := NewLocalNode(conf)
			node.state.Set(tc.state)
			previous := time.Now().Add(-time.Hour)
			node.lastStabilized.Store(previous)

			peer := new(mocks.VNode)
			peer.On("ID").Return(uint64(30))
			peer.On("Identity").Return(&protocol.Node{Id: 30}).Maybe()
			peer.On("GetPredecessor").Return(nil, tc.probeErr).Once()
			peer.On("GetSuccessors").Return(nil, tc.probeErr).Once()
			if tc.probeErr == nil && tc.state != chord.Leaving {
				peer.On("Notify", node).Return(tc.notifyErr).Once()
			}
			node.successors = []chord.VNode{peer}
			if tc.failedHead {
				dead := new(mocks.VNode)
				dead.On("ID").Return(uint64(20))
				dead.On("Identity").Return(&protocol.Node{Id: 20})
				dead.On("GetPredecessor").Return(nil, failed).Once()
				dead.On("GetSuccessors").Return(nil, failed).Once()
				node.successors = append([]chord.VNode{dead}, node.successors...)
				defer dead.AssertExpectations(t)
			}
			node.succListHash.Store(node.hash(node.successors))

			started := time.Now()
			err := node.stabilize()
			as.ErrorIs(err, tc.wantErr)
			if tc.wantTimestamp {
				as.WithinRange(node.lastStabilized.Load(), started, time.Now())
			} else {
				as.Equal(previous, node.lastStabilized.Load())
			}
			// A successful refresh survives notification failure; exhausted
			// probes retain the previous list for the next maintenance pass.
			as.Equal([]chord.VNode{peer}, node.getSuccessors())
			peer.AssertExpectations(t)
		})
	}
}
