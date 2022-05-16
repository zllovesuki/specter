package node

import (
	"crypto/rand"
	mathRand "math/rand"
	"specter/kv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func makeRing(as *assert.Assertions, num int) ([]*LocalNode, func()) {
	nodes := make([]*LocalNode, num)
	for i := 0; i < num; i++ {
		node := NewLocalNode(DevConfig(as))
		nodes[i] = node
	}

	nodes[0].Create()
	for i := 1; i < num; i++ {
		nodes[i].Join(nodes[0])
		<-time.After(time.Millisecond * 200)
	}

	<-time.After(time.Millisecond * 1000)

	RingCheck(as, nodes, true)

	return nodes, func() {
		for i := 0; i < num; i++ {
			nodes[i].Stop()
		}
		<-time.After(time.Millisecond * 100)
	}
}

func TestKVOperation(t *testing.T) {
	as := assert.New(t)

	nodes, done := makeRing(as, 5)
	defer done()

	key := make([]byte, 16)
	rand.Read(key)

	for _, local := range nodes {
		value := make([]byte, 16)

		rand.Read(value)

		// Put
		err := local.Put(key, value)
		as.Nil(err)

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

func TestKeyTransferOut(t *testing.T) {
	as := assert.New(t)

	numNodes := 7
	nodes, done := makeRing(as, numNodes)
	defer done()

	keys, values := makeKV(200, 32)

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
	<-time.After(time.Millisecond * 500)

	for _, node := range nodes {
		if node == randomNode {
			continue
		}
		pre := node.getPredecessor()
		as.True(node.kv.(*kv.MemoryMap).Fsck(pre.ID(), node.ID()), "node %d contains out of range keys", node.ID())
	}

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
	as := assert.New(t)

	numNodes := 2
	nodes, done := makeRing(as, numNodes)
	defer done()

	keys, values := makeKV(1000, 48)

	for i := range keys {
		as.Nil(nodes[0].Put(keys[i], values[i]))
	}

	newNode := NewLocalNode(DevConfig(as))
	newNode.Join(nodes[0])

	<-time.After(time.Millisecond * 500)

	t.Logf("yeet")
}
