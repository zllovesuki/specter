package chord

import (
	"context"
	"testing"

	"go.miragespace.co/specter/spec/chord"

	"github.com/stretchr/testify/require"
)

func TestJoinAfterMissingPredecessor(t *testing.T) {
	as := require.New(t)
	ctx := context.Background()
	key, value := []byte("transferred"), []byte("move")

	nodeConfig := devConfig(t, as)
	nodeConfig.Identity.Id = chord.Hash([]byte("owner"))
	node := NewLocalNode(nodeConfig)
	joinerConfig := devConfig(t, as)
	joinerConfig.Identity.Id = chord.Hash(key)
	joiner := NewLocalNode(joinerConfig)
	as.NotEqual(node.ID(), joiner.ID())

	// Keep maintenance tasks stopped so they cannot repair the missing
	// predecessor before the join request exercises the temporary gap.
	node.successors = []chord.VNode{node}
	node.succListHash.Store(node.hash(node.successors))
	for i := 1; i <= chord.MaxFingerEntries; i++ {
		node.fingers[i].node = node
	}
	node.state.Set(chord.Active)
	as.NoError(node.kv.Put(ctx, key, value))

	predecessor, successors, err := node.RequestToJoin(joiner)
	as.ErrorIs(err, chord.ErrJoinInvalidState)
	as.True(chord.ErrorIsRetryable(err))
	as.Nil(predecessor)
	as.Nil(successors)
	as.Equal(chord.Active, node.state.Get())
	as.Nil(node.getPredecessor())
	as.Nil(node.surrogate)

	stored, err := node.kv.Get(ctx, key)
	as.NoError(err)
	as.Equal(value, stored)
	keys, err := joiner.kv.RangeKeys(ctx, 0, 0)
	as.NoError(err)
	as.Empty(keys)

	// A subsequent notification establishes the range boundary and allows a
	// normal join, including transfer and membership-lock release, to finish.
	as.NoError(node.Notify(node))
	as.NoError(joiner.Join(node))
	defer joiner.Leave()
	as.Equal(chord.Active, node.state.Get())
	as.Equal(chord.Active, joiner.state.Get())

	keys, err = node.kv.RangeKeys(ctx, 0, 0)
	as.NoError(err)
	as.Empty(keys)
	stored, err = joiner.kv.Get(ctx, key)
	as.NoError(err)
	as.Equal(value, stored)
}
