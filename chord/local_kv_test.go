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

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/util/testcond"

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

		rand.Read(key)
		rand.Read(value)

		// Put
		err := local.Put(context.Background(), key, value)
		as.NoError(err)

		fsck(as, nodes)

		// Get
		for _, remote := range nodes {
			r, err := remote.Get(context.Background(), key)
			as.NoError(err)
			as.EqualValues(value, r)
		}

		// Overwrite
		rand.Read(value)
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
		rand.Read(value)
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

	keys := kv.RangeKeys(0, 0)
	for _, key := range keys {
		if !chord.BetweenInclusiveHigh(low, chord.Hash(key), high) {
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

	mathRand.Seed(time.Now().UnixNano())
	randomNode := nodes[mathRand.Intn(numNodes)]

	successor := randomNode.getSuccessor()
	predecessor := randomNode.getPredecessor()
	t.Logf("precedessor: %d, leaving: %d, successor: %d", predecessor.ID(), randomNode.ID(), successor.ID())

	leavingKeys := randomNode.kv.RangeKeys(0, 0)

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

	succVals := successor.(*LocalNode).kv.Export(leavingKeys)
	as.Len(succVals, len(leavingKeys))

	indicies := make([]int, 0)
	for _, k := range leavingKeys {
		for i := range keys {
			if string(keys[i]) == string(k) {
				indicies = append(indicies, i)
			}
		}
	}
	as.Len(indicies, len(leavingKeys))

	for i, v := range succVals {
		as.EqualValues(values[indicies[i]], v.GetSimpleValue())
	}

	preVals := predecessor.(*LocalNode).kv.Export(leavingKeys)
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

	<-time.After(waitInterval * 2)

	keys = n1.kv.RangeKeys(0, 0)
	as.Greater(len(keys), 0)
	vals := n1.kv.Export(keys)
	for _, val := range vals {
		as.Greater(len(val.GetSimpleValue()), 0)
	}

	fsck(as, []*LocalNode{n1, seed})

	n2Cfg := devConfig(t, as)
	n2Cfg.Identity.Id = (chord.MaxIdentitifer / 4 * 3) // 3 quarter
	n2 := NewLocalNode(n2Cfg)
	as.NoError(n2.Join(seed))
	defer n2.Leave()

	keys = n2.kv.RangeKeys(0, 0)
	as.Greater(len(keys), 0)
	vals = n2.kv.Export(keys)
	for _, val := range vals {
		as.Greater(len(val.GetSimpleValue()), 0)
	}

	fsck(as, []*LocalNode{n2, n1, seed})
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

	nodes[0].Logger.Debug("Starting test validation")

	found := 0
	missingIndicies := make([]int, 0)
	mismatchedIndicies := make([]int, 0)
	for i := range keys {
		val, err := nodes[0].Get(context.Background(), keys[i])
		as.NoError(err)

		if bytes.Equal(values[i], val) {
			found++
		} else if len(val) == 0 {
			missingIndicies = append(missingIndicies, i)
		} else {
			mismatchedIndicies = append(mismatchedIndicies, i)
		}
	}

	t.Logf("stale ownership counts: %d", stale)
	t.Logf("missing indicies: %+v\n", missingIndicies)
	t.Logf("mismatched indicies: %+v\n", mismatchedIndicies)

	if len(missingIndicies) > 0 {
		for _, i := range missingIndicies {
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

	as.Equal(numKeys, found, "expect %d keys to be found, but only %d keys found with %d missing and %d mismatched", numKeys, found, len(missingIndicies), len(mismatchedIndicies))

	for i := numNodes - 1; i >= 0; i-- {
		nodes[i].Leave()
	}
	k := nodes[0].kv.RangeKeys(0, 0)
	as.Equal(numKeys, len(k), "expect %d keys to be found on the remaining node, but only %d keys found", numKeys, len(k))
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
		}
	}()

	// kill every node except the first node
	for i := 1; i < numNodes; i++ {
		nodes[i].Leave()
	}

	<-syncA

	nodes[0].Logger.Debug("Starting test validation")

	found := 0
	missingIndicies := make([]int, 0)
	mismatchedIndicies := make([]int, 0)
	for i := range keys {
		// all keys should be in the first node
		val, err := nodes[0].kv.Get(context.Background(), keys[i])
		as.NoError(err)
		if bytes.Equal(values[i], val) {
			found++
		} else if len(val) == 0 {
			missingIndicies = append(missingIndicies, i)
		} else {
			mismatchedIndicies = append(mismatchedIndicies, i)
		}
	}

	t.Logf("stale ownership counts: %d", stale)
	t.Logf("missing indicies: %+v\n", missingIndicies)
	t.Logf("mismatched indicies: %+v\n", mismatchedIndicies)

	if len(missingIndicies) > 0 {
		for _, i := range missingIndicies {
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

	if len(mismatchedIndicies) > 0 {
		for _, i := range mismatchedIndicies {
			k := keys[i]
			for j := 1; j < numNodes; j++ {
				v, _ := nodes[j].kv.Get(context.Background(), k)
				if v != nil {
					t.Logf("mismatched key index %d found in node %d", i, nodes[j].ID())
				}
			}
		}
	}

	as.Equal(numKeys, found, "expect %d keys to be found, but only %d keys found with %d missing and %d mismatched", numKeys, found, len(missingIndicies), len(mismatchedIndicies))

	k := nodes[0].kv.RangeKeys(0, 0)
	as.Equal(numKeys, len(k), "expect %d keys to be found on the remaining node, but only %d keys found", numKeys, len(k))
}
