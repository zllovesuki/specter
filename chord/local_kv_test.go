package chord

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	mathRand "math/rand"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/chord"

	"github.com/stretchr/testify/require"
)

func makeKV(num int, length int) (keys [][]byte, values [][]byte) {
	keys = make([][]byte, num)
	values = make([][]byte, num)

	for i := range keys {
		keys[i] = make([]byte, length)
		values[i] = make([]byte, length)
		rand.Read(keys[i])
		rand.Read(values[i])
	}
	return
}

func TestKVOperation(t *testing.T) {
	as := require.New(t)

	nodes, done := makeRing(as, 5)
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
	nodes, done := makeRing(as, numNodes)
	defer done()

	keys, values := makeKV(30, 8)

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

	seedCfg := devConfig(as)
	seedCfg.Identity.Id = chord.MaxIdentitifer / 2 // halfway
	seed := NewLocalNode(seedCfg)
	as.NoError(seed.Create())
	defer seed.Leave()
	waitRing(as, seed)

	keys, values := makeKV(400, 8)

	for i := range keys {
		err := seed.Put(context.Background(), keys[i], values[i])
		as.NoError(err)
	}

	n1Cfg := devConfig(as)
	n1Cfg.Identity.Id = chord.MaxIdentitifer / 4 // 1 quarter
	n1 := NewLocalNode(n1Cfg)
	as.NoError(n1.Join(seed))
	defer n1.Leave()
	waitRing(as, n1)

	<-time.After(waitInterval * 2)

	keys = n1.kv.RangeKeys(0, 0)
	as.Greater(len(keys), 0)
	vals := n1.kv.Export(keys)
	for _, val := range vals {
		as.Greater(len(val.GetSimpleValue()), 0)
	}

	fsck(as, []*LocalNode{n1, seed})

	n2Cfg := devConfig(as)
	n2Cfg.Identity.Id = (chord.MaxIdentitifer / 4 * 3) // 3 quarter
	n2 := NewLocalNode(n2Cfg)
	as.NoError(n2.Join(seed))
	defer n2.Leave()
	waitRing(as, n2)

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
		t.Run(fmt.Sprintf("test with %d nodes and %d keys", tc.numNodes, tc.numKeys), func(t *testing.T) {
			concurrentJoinKVOps(t, tc.numNodes, tc.numKeys)
		})
	}
}

func concurrentJoinKVOps(t *testing.T, numNodes, numKeys int) {
	as := require.New(t)

	// can't use makeRing here as we need to manually control joining
	nodes := make([]*LocalNode, numNodes)
	for i := 0; i < numNodes; i++ {
		node := NewLocalNode(devConfig(as))
		nodes[i] = node
	}

	keys, values := makeKV(numKeys, 16)
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
		}
	}()

	for i := 1; i < numNodes; i++ {
		as.NoError(nodes[i].Join(nodes[0]))
	}

	<-syncA

	nodes[0].Logger.Debug("Starting test validation")

	found := 0
	missing := 0
	indices := make([]int, 0)
	for i := range keys {
	RETRY:
		val, err := nodes[0].Get(context.Background(), keys[i])
		if err != nil {
			if chord.ErrorIsRetryable(err) {
				t.Logf("[get] outdated ownership at key %d", i)
				time.Sleep(defaultInterval)
				goto RETRY
			}
			as.NoError(err)
			return
		}
		if bytes.Equal(values[i], val) {
			found++
		} else {
			missing++
			indices = append(indices, i)
		}
	}

	defer func() {
		<-time.After(waitInterval)
		t.Logf("stale ownership counts: %d", stale)
		t.Logf("missing indicies: %+v\n", indices)
	}()
	as.Equal(numKeys, found, "expect %d keys to be found, but only %d keys found with %d missing", numKeys, found, missing)

	for i := 0; i < numNodes; i++ {
		nodes[i].Leave()
	}
}

func TestConcurrentLeaveKV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping many nodes concurrent leave kv in short mode")
	}

	for _, tc := range concurrentParams {
		t.Run(fmt.Sprintf("test with %d nodes and %d keys", tc.numNodes, tc.numKeys), func(t *testing.T) {
			concurrentLeaveKVOps(t, tc.numNodes, tc.numKeys)
		})
	}
}

func concurrentLeaveKVOps(t *testing.T, numNodes, numKeys int) {
	as := require.New(t)

	nodes, done := makeRing(as, numNodes)
	defer done()

	keys, values := makeKV(numKeys, 16)
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
	missing := 0
	indices := make([]int, 0)
	for i := range keys {
		// all keys should be in the first node
		val, err := nodes[0].kv.Get(context.Background(), keys[i])
		as.NoError(err)
		if bytes.Equal(values[i], val) {
			found++
		} else {
			missing++
			indices = append(indices, i)
		}
	}
	defer func() {
		<-time.After(waitInterval)
		t.Logf("stale ownership counts: %d", stale)
		t.Logf("missing indicies: %+v\n", indices)
	}()

	if len(indices) > 0 {
		for _, i := range indices {
			k := keys[i]
			for j := 1; j < numNodes; j++ {
				v, _ := nodes[j].kv.Get(context.Background(), k)
				if v != nil {
					t.Logf("missing key index %d found in node %d", i, nodes[j].ID())
				}
			}
		}
	}

	k := nodes[0].kv.RangeKeys(0, 0)
	as.Equal(numKeys, len(k), "expect %d keys to be found on the remaining node, but only %d keys found", numKeys, len(k))
	as.Equal(numKeys, found, "expect %d keys to be found, but only %d keys found with %d missing", numKeys, found, missing)
}
