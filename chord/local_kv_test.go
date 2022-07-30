package chord

import (
	"bytes"
	"crypto/rand"
	"fmt"
	mathRand "math/rand"
	"testing"
	"time"

	"kon.nect.sh/specter/kv"

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
		err := local.Put(key, value)
		as.NoError(err)

		fsck(as, nodes)

		// Get
		for _, remote := range nodes {
			r, err := remote.Get(key)
			as.NoError(err)
			as.EqualValues(value, r)
		}

		// Overwrite
		rand.Read(value)
		err = local.Put(key, value)
		as.NoError(err)
		for _, remote := range nodes {
			r, err := remote.Get(key)
			as.Nil(err)
			as.EqualValues(value, r)
		}

		// Delete
		err = local.Delete(key)
		as.NoError(err)
		for _, remote := range nodes {
			r, err := remote.Get(key)
			as.Nil(err)
			as.Nil(r)
		}
	}
}

func fsck(as *require.Assertions, nodes []*LocalNode) {
	for _, node := range nodes {
		pre := node.getPredecessor()
		as.True(node.kv.(*kv.MemoryMap).Fsck(pre.ID(), node.ID()), "node %d contains out of range keys", node.ID())
	}
}

func TestKeyTransferOut(t *testing.T) {
	as := require.New(t)

	numNodes := 3
	nodes, done := makeRing(as, numNodes)
	defer done()

	keys, values := makeKV(30, 8)

	for i := range keys {
		as.Nil(nodes[0].Put(keys[i], values[i]))
	}

	mathRand.Seed(time.Now().UnixNano())
	randomNode := nodes[mathRand.Intn(numNodes)]

	successor := randomNode.getSuccessor()
	predecessor := randomNode.getPredecessor()
	t.Logf("precedessor: %d, leaving: %d, successor: %d", predecessor.ID(), randomNode.ID(), successor.ID())

	leavingKeys, err := randomNode.kv.LocalKeys(0, 0)
	as.NoError(err)

	randomNode.Stop()
	<-time.After(waitInterval)

	c := make([]*LocalNode, 0)
	for _, node := range nodes {
		if node == randomNode {
			continue
		}
		c = append(c, node)
	}
	fsck(as, c)

	succVals, err := successor.LocalGets(leavingKeys)
	as.NoError(err)
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
		as.EqualValues(values[indicies[i]], v)
	}

	preVals, err := predecessor.LocalGets(leavingKeys)
	as.NoError(err)
	for _, v := range preVals {
		as.Nil(v)
	}
}

// Even with 300 keys we could still run into the issues of
// all the keys fall into just 1 node and when new node joins,
// no keys will be transferred (see line denoted #OFFEND below).
func TestKeyTransferIn(t *testing.T) {
	as := require.New(t)

	numNodes := 1
	nodes, done := makeRing(as, numNodes)
	defer done()

	keys, values := makeKV(300, 8)

	for i := range keys {
		err := nodes[0].Put(keys[i], values[i])
		as.NoError(err)
	}

	n1 := NewLocalNode(devConfig(as))
	as.NoError(n1.Join(nodes[0]))
	defer n1.Stop()

	<-time.After(waitInterval * 2)

	keys, err := n1.LocalKeys(0, 0)
	as.NoError(err)
	as.Greater(len(keys), 0)
	vals, err := n1.LocalGets(keys)
	as.NoError(err)
	for _, val := range vals {
		as.Greater(len(val), 0)
	}

	fsck(as, []*LocalNode{n1, nodes[0]})

	n2 := NewLocalNode(devConfig(as))
	as.NoError(n2.Join(nodes[0]))
	defer n2.Stop()

	<-time.After(waitInterval * 2)

	keys, err = n2.LocalKeys(0, 0)
	as.NoError(err)
	as.Greater(len(keys), 0) // #OFFEND
	vals, err = n2.LocalGets(keys)
	as.NoError(err)
	for _, val := range vals {
		as.Greater(len(val), 0)
	}

	fsck(as, []*LocalNode{n2, n1, nodes[0]})
}

func TestConcurrentJoinKV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping many nodes concurrent join kv in short mode")
	}

	type test struct {
		numNodes int
		numKeys  int
	}

	tests := []test{
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
	for _, tc := range tests {
		t.Run(fmt.Sprintf("test with %d nodes and %d keys", tc.numNodes, tc.numKeys), func(t *testing.T) {
			concurrentJoinKVOps(t, tc.numNodes, tc.numKeys)
		})
	}

}

func concurrentJoinKVOps(t *testing.T, numNodes, numKeys int) {
	as := require.New(t)

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
			err := nodes[0].Put(keys[i], values[i])
			if err == ErrStateOwnership {
				stale++
				t.Logf("outdated ownership at key %d", i)
				<-time.After(defaultInterval)
				goto RETRY
			}
			t.Logf("message %d inserted\n", i)
			<-time.After(defaultInterval)
		}
	}()

	for i := 1; i < numNodes; i++ {
		nodes[i].Join(nodes[0])
		<-time.After(waitInterval)
	}

	<-syncA

	found := 0
	missing := 0
	indices := make([]int, 0)
	for i := range keys {
		val, err := nodes[0].Get(keys[i])
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
	as.Equal(numKeys, found, "expect %d keys to be found, but only %d keys found with %d missing", numKeys, found, missing)

	for i := 0; i < numNodes; i++ {
		nodes[i].Stop()
	}
}

func TestConcurrentLeaveKV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping many nodes concurrent leave kv in short mode")
	}

	type test struct {
		numNodes int
		numKeys  int
	}

	tests := []test{
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
	for _, tc := range tests {
		t.Run(fmt.Sprintf("test with %d nodes and %d keys", tc.numNodes, tc.numKeys), func(t *testing.T) {
			concurrentLeaveKVOps(t, tc.numNodes, tc.numKeys)
		})
	}
}

func concurrentLeaveKVOps(t *testing.T, numNodes, numKeys int) {
	as := require.New(t)

	nodes := make([]*LocalNode, numNodes)
	for i := 0; i < numNodes; i++ {
		node := NewLocalNode(devConfig(as))
		nodes[i] = node
	}

	keys, values := makeKV(numKeys, 16)
	syncA := make(chan struct{})

	nodes[0].Create()
	defer nodes[0].Stop()

	for i := 1; i < numNodes; i++ {
		nodes[i].Join(nodes[0])
		<-time.After(waitInterval)
	}

	// allow plenty of time for ring to stablize
	<-time.After(time.Second)

	stale := 0
	go func() {
		defer close(syncA)

		for i := range keys {
		RETRY:
			err := nodes[0].Put(keys[i], values[i])
			if err == ErrStateOwnership {
				stale++
				t.Logf("outdated ownership at key %d", i)
				<-time.After(defaultInterval)
				goto RETRY
			}
			t.Logf("message %d inserted\n", i)
			<-time.After(defaultInterval)
		}
	}()

	// kill every node except the first node
	for i := 1; i < numNodes; i++ {
		nodes[i].Stop()
	}

	<-syncA

	found := 0
	missing := 0
	indices := make([]int, 0)
	for i := range keys {
		// inspec
		val, err := nodes[0].Get(keys[i])
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
	as.Equal(numKeys, found, "expect %d keys to be found, but only %d keys found with %d missing", numKeys, found, missing)

}
