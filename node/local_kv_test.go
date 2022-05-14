package node

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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

	RingCheck(as, nodes)

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
