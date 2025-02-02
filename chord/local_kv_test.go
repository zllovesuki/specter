package chord

import (
	"bytes"
	"container/ring"
	"context"
	"crypto/rand"
	"fmt"
	mathRand "math/rand"
	"sort"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/util/testcond"

	"github.com/stretchr/testify/require"
)

func makeKV(as *require.Assertions, num int, length int) (keys [][]byte, values [][]byte) {
	keys = make([][]byte, num)
	values = make([][]byte, num)

	var err error
	for i := range keys {
		keys[i] = make([]byte, length)
		values[i] = make([]byte, length)
		l := copy(keys[i], []byte(fmt.Sprintf("key %d: ", i)))
		_, err = rand.Read(keys[i][l:])
		as.NoError(err)
		_, err = rand.Read(values[i])
		as.NoError(err)
	}
	return
}

func TestKVOperation(t *testing.T) {
	as := require.New(t)

	nodes, done := makeRing(t, as, 5)
	defer done()

	key := make([]byte, 16)

	for _, local := range nodes {
		value := make([]byte, 16)

		_, err := rand.Read(key)
		as.NoError(err)
		_, err = rand.Read(value)
		as.NoError(err)

		// Put
		err = local.Put(context.Background(), key, value)
		as.NoError(err)

		fsck(as, nodes)

		// Get
		for _, remote := range nodes {
			r, err := remote.Get(context.Background(), key)
			as.NoError(err)
			as.EqualValues(value, r)
		}

		// Overwrite
		_, err = rand.Read(value)
		as.NoError(err)
		err = local.Put(context.Background(), key, value)
		as.NoError(err)
		for _, remote := range nodes {
			r, err := remote.Get(context.Background(), key)
			as.NoError(err)
			as.EqualValues(value, r)
		}

		// Delete
		err = local.Delete(context.Background(), key)
		as.NoError(err)
		for _, remote := range nodes {
			r, err := remote.Get(context.Background(), key)
			as.NoError(err)
			as.Nil(r)
		}

		// PrefixAppend
		_, err = rand.Read(value)
		as.NoError(err)
		err = local.PrefixAppend(context.Background(), key, value)
		as.NoError(err)
		err = local.PrefixAppend(context.Background(), key, value)
		as.ErrorIs(err, chord.ErrKVPrefixConflict)
		for _, remote := range nodes {
			// PrefixList
			ret, err := remote.PrefixList(context.Background(), key)
			as.NoError(err)
			as.Len(ret, 1)
			as.EqualValues(value, ret[0])
		}

		// PrefixRemove
		err = local.PrefixRemove(context.Background(), key, value)
		as.NoError(err)
		for _, remote := range nodes {
			// PrefixList
			ret, err := remote.PrefixList(context.Background(), key)
			as.NoError(err)
			as.Len(ret, 0)
		}
		err = local.PrefixAppend(context.Background(), key, value)
		as.NoError(err)
	}
}

func kvFsck(kv chord.KVProvider, low, high uint64) bool {
	valid := true

	keys, _ := kv.RangeKeys(context.Background(), 0, 0)
	for _, key := range keys {
		if !chord.Between(low, chord.Hash(key), high, true) {
			valid = false
		}
	}
	return valid
}

func fsck(as *require.Assertions, nodes []*LocalNode) {
	for _, node := range nodes {
		pre := node.getPredecessor()
		as.True(kvFsck(node.kv, pre.ID(), node.ID()), "node %d contains out of range keys", node.ID())
	}
}

func TestKeyTransferOut(t *testing.T) {
	as := require.New(t)

	numNodes := 3
	nodes, done := makeRing(t, as, numNodes)
	defer done()

	keys, values := makeKV(as, 30, 8)

	for i := range keys {
		as.Nil(nodes[0].Put(context.Background(), keys[i], values[i]))
	}

	randomNode := nodes[mathRand.Intn(numNodes)]

	successor := randomNode.getSuccessor()
	predecessor := randomNode.getPredecessor()
	t.Logf("predecessor: %d, leaving: %d, successor: %d", predecessor.ID(), randomNode.ID(), successor.ID())

	leavingKeys, err := randomNode.kv.RangeKeys(context.Background(), 0, 0)
	as.NoError(err)

	randomNode.Leave()
	<-time.After(waitInterval)

	c := make([]*LocalNode, 0)
	for _, node := range nodes {
		if node == randomNode {
			continue
		}
		c = append(c, node)
	}
	fsck(as, c)

	succVals, _ := successor.(*LocalNode).kv.Export(context.Background(), leavingKeys)
	as.Len(succVals, len(leavingKeys))

	indices := make([]int, 0)
	for _, k := range leavingKeys {
		for i := range keys {
			if string(keys[i]) == string(k) {
				indices = append(indices, i)
			}
		}
	}
	as.Len(indices, len(leavingKeys))

	for i, v := range succVals {
		as.EqualValues(values[indices[i]], v.GetSimpleValue())
	}

	preVals, _ := predecessor.(*LocalNode).kv.Export(context.TODO(), leavingKeys)
	for _, v := range preVals {
		as.Nil(v.GetSimpleValue())
	}
}

func TestKeyTransferIn(t *testing.T) {
	as := require.New(t)

	seedCfg := devConfig(t, as)
	seedCfg.Identity.Id = chord.MaxIdentitifer / 2 // halfway
	seed := NewLocalNode(seedCfg)
	as.NoError(seed.Create())
	defer seed.Leave()
	waitRing(as, seed)

	keys, values := makeKV(as, 400, 8)

	for i := range keys {
		err := seed.Put(context.Background(), keys[i], values[i])
		as.NoError(err)
	}

	n1Cfg := devConfig(t, as)
	n1Cfg.Identity.Id = chord.MaxIdentitifer / 4 // 1 quarter
	n1 := NewLocalNode(n1Cfg)
	as.NoError(n1.Join(seed))
	defer n1.Leave()
	waitRing(as, n1)

	<-time.After(waitInterval * 2)

	keys, _ = n1.kv.RangeKeys(context.Background(), 0, 0)
	as.Greater(len(keys), 0)
	vals, _ := n1.kv.Export(context.Background(), keys)
	for _, val := range vals {
		as.Greater(len(val.GetSimpleValue()), 0)
	}

	fsck(as, []*LocalNode{n1, seed})

	n2Cfg := devConfig(t, as)
	n2Cfg.Identity.Id = (chord.MaxIdentitifer / 4 * 3) // 3 quarter
	n2 := NewLocalNode(n2Cfg)
	as.NoError(n2.Join(seed))
	defer n2.Leave()
	waitRing(as, n2)

	keys, _ = n2.kv.RangeKeys(context.Background(), 0, 0)
	as.Greater(len(keys), 0)
	vals, _ = n2.kv.Export(context.Background(), keys)
	for _, val := range vals {
		as.Greater(len(val.GetSimpleValue()), 0)
	}

	fsck(as, []*LocalNode{n2, n1, seed})
}

func TestListKeys(t *testing.T) {
	as := require.New(t)

	numNodes := 3
	nodes, done := makeRing(t, as, numNodes)
	defer done()

	keys, values := makeKV(as, 30, 8)

	for i := range keys {
		as.Nil(nodes[0].Put(context.Background(), keys[i], values[i]))
	}

	composite, err := nodes[0].ListKeys(context.Background(), []byte(""))
	as.NoError(err)
	as.Len(composite, len(keys))
	found := 0
	for _, k1 := range composite {
		for _, k2 := range keys {
			if bytes.Equal(k1.GetKey(), k2) {
				found++
			}
		}
	}
	as.Equal(len(keys), found)
}

type concurrentTest struct {
	numNodes int
	numKeys  int
}

var concurrentParams = []concurrentTest{
	// 64
	{
		numNodes: 64,
		numKeys:  100,
	},
	{
		numNodes: 64,
		numKeys:  200,
	},
	{
		numNodes: 64,
		numKeys:  300,
	},
	{
		numNodes: 64,
		numKeys:  600,
	},
	// 128
	{
		numNodes: 128,
		numKeys:  100,
	},
	{
		numNodes: 128,
		numKeys:  200,
	},
	{
		numNodes: 128,
		numKeys:  300,
	},
	{
		numNodes: 128,
		numKeys:  600,
	},
}

func TestConcurrentJoinKV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping many nodes concurrent join kv in short mode")
	}

	for _, tc := range concurrentParams {
		tc := tc
		t.Run(fmt.Sprintf("test with %d nodes and %d keys", tc.numNodes, tc.numKeys), func(t *testing.T) {
			t.Parallel()
			concurrentJoinKVOps(t, tc.numNodes, tc.numKeys)
		})
	}
}

func awaitStablizedGlobally(t *testing.T, as *require.Assertions, timeout time.Duration, nodes []*LocalNode) {
	// create a sorted list of test nodes
	refNodes := append([]*LocalNode{}, nodes...)
	sort.SliceStable(refNodes, func(i, j int) bool {
		return refNodes[i].ID() < refNodes[j].ID()
	})
	// use container/ring to build the stablized list of successors for each node
	fullRing := ring.New(len(nodes))
	succsMap := make(map[uint64]*ring.Ring)
	for _, n := range refNodes {
		fullRing.Value = n.ID()
		fullRing = fullRing.Next()
		succsMap[n.ID()] = fullRing
	}
	// then we check for condition, where every node has the correct local states
	// with respect to global order of the ring
	as.NoError(testcond.WaitForCondition(func() bool {
		for _, node := range nodes {
			if node.getPredecessor() == nil {
				return false
			}
			if node.getSuccessor() == nil {
				return false
			}
		}
		// counter clockwise
		for i := 0; i < len(refNodes)-1; i++ {
			if refNodes[i].ID() != refNodes[i+1].getPredecessor().ID() {
				return false
			}
		}
		if refNodes[len(refNodes)-1].ID() != refNodes[0].getPredecessor().ID() {
			return false
		}
		// clockwise
		for i := 0; i < len(nodes)-1; i++ {
			if refNodes[i+1].ID() != refNodes[i].getSuccessor().ID() {
				return false
			}
		}
		if refNodes[0].ID() != refNodes[len(refNodes)-1].getSuccessor().ID() {
			return false
		}
		// ensure that successor list is correct (eventual consistency),
		// otherwise lookup may go to the wrong node and failing the test incorrectly
		for _, n := range nodes {
			expectSuccs := succsMap[n.ID()]
			actualSuccs := n.getSuccessors()
			for _, s := range actualSuccs {
				if s.ID() != expectSuccs.Value {
					return false
				}
				expectSuccs = expectSuccs.Next()
			}
		}
		t.Logf("[stablized] valid ring:\n")
		fullRing.Do(func(a any) {
			t.Logf("  %v\n", a)
		})
		return true
	}, waitInterval, timeout))
}

func concurrentJoinKVOps(t *testing.T, numNodes, numKeys int) {
	as := require.New(t)

	// can't use makeRing here as we need to manually control joining
	nodes := make([]*LocalNode, numNodes)
	for i := 0; i < numNodes; i++ {
		node := NewLocalNode(devConfig(t, as))
		nodes[i] = node
	}

	keys, values := makeKV(as, numKeys, 64)
	syncA := make(chan struct{})

	nodes[0].Create()

	stale := 0
	go func() {
		defer close(syncA)

		for i := range keys {
		RETRY:
			err := nodes[0].Put(context.Background(), keys[i], values[i])
			if err != nil {
				if chord.ErrorIsRetryable(err) {
					stale++
					t.Logf("[put] outdated ownership at key %d", i)
					time.Sleep(defaultInterval)
					goto RETRY
				}
				as.NoError(err)
				return
			}
			t.Logf("message %d inserted\n", i)
			time.Sleep(defaultInterval) // used to pace insertions
		}
	}()

	for i := 1; i < numNodes; i++ {
		as.NoError(nodes[i].Join(nodes[0]))
	}

	<-syncA

	// wait until the ring is fully stablized before we check for missing values
	awaitStablizedGlobally(t, as, time.Second*10, nodes)

	nodes[0].logger.Debug("Starting test validation")

	found := 0
	missingIndices := make([]int, 0)
	mismatchedIndices := make([]int, 0)
	for i := range keys {
		val, err := nodes[0].Get(context.Background(), keys[i])
		as.NoError(err)

		if bytes.Equal(values[i], val) {
			found++
		} else if len(val) == 0 {
			missingIndices = append(missingIndices, i)
		} else {
			mismatchedIndices = append(mismatchedIndices, i)
		}
	}

	t.Logf("stale ownership counts: %d", stale)
	t.Logf("missing indices: %+v\n", missingIndices)
	t.Logf("mismatched indices: %+v\n", mismatchedIndices)

	if len(missingIndices) > 0 {
		for _, i := range missingIndices {
			k := keys[i]
			got, err := nodes[0].FindSuccessor(chord.Hash(k))
			as.NoError(err)
			as.NotNil(got)
			for j := 1; j < numNodes; j++ {
				v, _ := nodes[j].kv.Get(context.Background(), k)
				if v != nil {
					t.Logf("missing key index %d routed to node %d but found in node %d", i, got.ID(), nodes[j].ID())
				}
			}
		}
	}

	as.Equal(numKeys, found, "expect %d keys to be found, but only %d keys found with %d missing and %d mismatched", numKeys, found, len(missingIndices), len(mismatchedIndices))

	for i := numNodes - 1; i >= 0; i-- {
		nodes[i].Leave()
	}

	lostIndices := make([]int, 0)
	k, _ := nodes[0].kv.RangeKeys(context.Background(), 0, 0)
	if len(k) != numKeys {
		for i := range keys {
			val, err := nodes[0].kv.Get(context.Background(), keys[i])
			as.NoError(err)

			if !bytes.Equal(values[i], val) {
				lostIndices = append(lostIndices, i)
			}
		}
	}
	t.Logf("lost indices: %+v\n", lostIndices)
	as.Equal(0, len(lostIndices), "expect no keys to be lost when one node remains, but %d keys were lost", len(lostIndices))
}

func TestConcurrentLeaveKV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping many nodes concurrent leave kv in short mode")
	}

	for _, tc := range concurrentParams {
		tc := tc
		t.Run(fmt.Sprintf("test with %d nodes and %d keys", tc.numNodes, tc.numKeys), func(t *testing.T) {
			t.Parallel()
			concurrentLeaveKVOps(t, tc.numNodes, tc.numKeys)
		})
	}
}

func concurrentLeaveKVOps(t *testing.T, numNodes, numKeys int) {
	as := require.New(t)

	nodes, done := makeRing(t, as, numNodes)
	defer done()

	// wait until the ring is fully stablized before we insert values
	awaitStablizedGlobally(t, as, time.Second*10, nodes)

	keys, values := makeKV(as, numKeys, 64)
	syncA := make(chan struct{})

	stale := 0
	go func() {
		defer close(syncA)

		for i := range keys {
		RETRY:
			err := nodes[0].Put(context.Background(), keys[i], values[i])
			if err != nil {
				if chord.ErrorIsRetryable(err) {
					stale++
					t.Logf("[put] outdated ownership at key %d", i)
					time.Sleep(defaultInterval)
					goto RETRY
				}
				as.NoError(err)
				return
			}
			t.Logf("message %d inserted\n", i)
			time.Sleep(defaultInterval) // used to pace insertions
		}
	}()

	// kill every node except the first node
	for i := 1; i < numNodes; i++ {
		nodes[i].Leave()
	}

	<-syncA

	nodes[0].logger.Debug("Starting test validation")

	found := 0
	missingIndices := make([]int, 0)
	mismatchedIndices := make([]int, 0)
	for i := range keys {
		// all keys should be in the first node
		val, err := nodes[0].kv.Get(context.Background(), keys[i])
		as.NoError(err)
		if bytes.Equal(values[i], val) {
			found++
		} else if len(val) == 0 {
			missingIndices = append(missingIndices, i)
		} else {
			mismatchedIndices = append(mismatchedIndices, i)
		}
	}

	t.Logf("stale ownership counts: %d", stale)
	t.Logf("missing indices: %+v\n", missingIndices)
	t.Logf("mismatched indices: %+v\n", mismatchedIndices)

	if len(missingIndices) > 0 {
		for _, i := range missingIndices {
			k := keys[i]
			got, err := nodes[0].FindSuccessor(chord.Hash(k))
			as.NoError(err)
			as.NotNil(got)
			for j := 1; j < numNodes; j++ {
				v, _ := nodes[j].kv.Get(context.Background(), k)
				if v != nil {
					t.Logf("missing key index %d routed to node %d but found in node %d", i, got.ID(), nodes[j].ID())
				}
			}
		}
	}

	if len(mismatchedIndices) > 0 {
		for _, i := range mismatchedIndices {
			k := keys[i]
			for j := 1; j < numNodes; j++ {
				v, _ := nodes[j].kv.Get(context.Background(), k)
				if v != nil {
					t.Logf("mismatched key index %d found in node %d", i, nodes[j].ID())
				}
			}
		}
	}

	as.Equal(numKeys, found, "expect %d keys to be found, but only %d keys found with %d missing and %d mismatched", numKeys, found, len(missingIndices), len(mismatchedIndices))

	lostIndices := make([]int, 0)
	k, _ := nodes[0].kv.RangeKeys(context.Background(), 0, 0)
	if len(k) != numKeys {
		for i := range keys {
			val, err := nodes[0].kv.Get(context.Background(), keys[i])
			as.NoError(err)

			if !bytes.Equal(values[i], val) {
				lostIndices = append(lostIndices, i)
			}
		}
	}
	t.Logf("lost indices: %+v\n", lostIndices)
	as.Equal(0, len(lostIndices), "expect no keys to be lost when one node remains, but %d keys were lost", len(lostIndices))
}
