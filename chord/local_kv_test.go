package chord

import (
	"crypto/rand"
	mathRand "math/rand"
	"testing"
	"time"

	"github.com/zllovesuki/specter/kv"

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

func makeRing(as *require.Assertions, num int) ([]*LocalNode, func()) {
	nodes := make([]*LocalNode, num)
	for i := 0; i < num; i++ {
		node := NewLocalNode(DevConfig(as))
		nodes[i] = node
	}

	nodes[0].Create()
	for i := 1; i < num; i++ {
		nodes[i].Join(nodes[0])
		<-time.After(waitInterval)
	}

	<-time.After(waitInterval)

	RingCheck(as, nodes, true)

	return nodes, func() {
		for i := 0; i < num; i++ {
			nodes[i].Stop()
		}
		<-time.After(waitInterval)
	}
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
		as.Nil(err)

		fsck(as, nodes)

		// Get
		for _, remote := range nodes {
			r, err := remote.Get(key)
			as.Nil(err)
			as.EqualValues(value, r)
		}

		// Overwrite
		rand.Read(value)
		err = local.Put(key, value)
		as.Nil(err)
		for _, remote := range nodes {
			r, err := remote.Get(key)
			as.Nil(err)
			as.EqualValues(value, r)
		}

		// Delete
		err = local.Delete(key)
		as.Nil(err)
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
	as.Nil(err)

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
	as.Nil(err)
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
	as.Nil(err)
	for _, v := range preVals {
		as.Nil(v)
	}
}

func TestKeyTransferIn(t *testing.T) {
	as := require.New(t)

	numNodes := 1
	nodes, done := makeRing(as, numNodes)
	defer done()

	keys, values := makeKV(200, 8)

	for i := range keys {
		as.Nil(nodes[0].Put(keys[i], values[i]))
	}

	n1 := NewLocalNode(DevConfig(as))
	n1.Join(nodes[0])
	defer n1.Stop()

	<-time.After(waitInterval)

	keys, err := n1.kv.LocalKeys(0, 0)
	as.Nil(err)
	as.Greater(len(keys), 0)
	vals, err := n1.kv.LocalGets(keys)
	as.Nil(err)
	for _, val := range vals {
		as.Greater(len(val), 0)
	}

	fsck(as, []*LocalNode{n1, nodes[0]})

	n2 := NewLocalNode(DevConfig(as))
	n2.Join(nodes[0])
	defer n2.Stop()

	<-time.After(waitInterval)

	keys, err = n2.kv.LocalKeys(0, 0)
	as.Nil(err)
	as.Greater(len(keys), 0)
	vals, err = n2.kv.LocalGets(keys)
	as.Nil(err)
	for _, val := range vals {
		as.Greater(len(val), 0)
	}

	fsck(as, []*LocalNode{n2, n1, nodes[0]})
}
