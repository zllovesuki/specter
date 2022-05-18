package node

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/zllovesuki/specter/kv"
	"github.com/zllovesuki/specter/spec/chord"
	"github.com/zllovesuki/specter/spec/protocol"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func DevConfig(as *assert.Assertions) NodeConfig {
	logger, err := zap.NewDevelopment()
	as.NoError(err)

	return NodeConfig{
		Logger: logger,
		Identity: &protocol.Node{
			Id: chord.Random(),
		},
		Transport:                &mockTransport{},
		KVProvider:               kv.WithChordHash(),
		StablizeInterval:         time.Millisecond * 50,
		FixFingerInterval:        time.Millisecond * 50,
		PredecessorCheckInterval: time.Millisecond * 50,
	}
}

func RingCheck(as *assert.Assertions, nodes []*LocalNode, counter bool) {
	if len(nodes) == 0 {
		return
	}
	for _, node := range nodes {
		as.NotNil(node.getPredecessor())
		as.NotNil(node.getSuccessor())
	}

	fmt.Printf("Ring: %s\n", nodes[0].RingTrace())

	if len(nodes) == 1 {
		as.Equal(nodes[0].ID(), nodes[0].getPredecessor().ID())
		as.Equal(nodes[0].ID(), nodes[0].getSuccessor().ID())
		return
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].ID() < nodes[j].ID()
	})
	if counter {
		// counter clockwise
		for i := 0; i < len(nodes)-1; i++ {
			as.Equal(nodes[i].ID(), nodes[i+1].getPredecessor().ID())
		}
		as.Equal(nodes[len(nodes)-1].ID(), nodes[0].getPredecessor().ID())
	}
	// clockwise
	for i := 0; i < len(nodes)-1; i++ {
		as.Equal(nodes[i+1].ID(), nodes[i].getSuccessor().ID())
	}
	as.Equal(nodes[0].ID(), nodes[len(nodes)-1].getSuccessor().ID())
}

func TestCreate(t *testing.T) {
	as := assert.New(t)

	n1 := NewLocalNode(DevConfig(as))
	n1.Create()

	<-time.After(time.Millisecond * 500)

	n1.Stop()

	<-time.After(time.Millisecond * 500)

	RingCheck(as, []*LocalNode{n1}, true)
}

func TestJoin(t *testing.T) {
	as := assert.New(t)

	n2 := NewLocalNode(DevConfig(as))
	n2.Create()

	n1 := NewLocalNode(DevConfig(as))
	as.Nil(n1.Join(n2))

	<-time.After(time.Millisecond * 500)

	n1.Stop()
	n2.Stop()

	<-time.After(time.Millisecond * 500)

	// skip counter clockwise check because we stopped first
	RingCheck(as, []*LocalNode{
		n1,
		n2,
	}, false)
}

func TestRandomNodes(t *testing.T) {
	as := assert.New(t)

	num := 20
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

	<-time.After(time.Millisecond * 500)

	RingCheck(as, nodes, true)

	for i := 0; i < num; i++ {
		nodes[i].Stop()
	}

	<-time.After(time.Millisecond * 500)

	for i := 0; i < num; i++ {
		as.Equal(nodes[i].getSuccessor().ID(), nodes[i].fingers[1].n.Load().(*atomicVNode).Node.ID())
		fmt.Printf("%d: %s\n---\n", nodes[i].ID(), nodes[i].FingerTrace())
	}
}
