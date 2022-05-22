package chord

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/zllovesuki/specter/kv"
	"github.com/zllovesuki/specter/spec/chord"
	"github.com/zllovesuki/specter/spec/protocol"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	defaultInterval = time.Millisecond * 10
	waitInterval    = defaultInterval * 10
)

func devConfig(as *require.Assertions) NodeConfig {
	logger, err := zap.NewDevelopment()
	as.NoError(err)

	return NodeConfig{
		Logger: logger,
		Identity: &protocol.Node{
			Id: chord.Random(),
		},
		Transport:                &mockTransport{},
		KVProvider:               kv.WithChordHash(),
		StablizeInterval:         defaultInterval,
		FixFingerInterval:        defaultInterval,
		PredecessorCheckInterval: defaultInterval,
	}
}

func makeRing(as *require.Assertions, num int) ([]*LocalNode, func()) {
	nodes := make([]*LocalNode, num)
	for i := 0; i < num; i++ {
		node := NewLocalNode(devConfig(as))
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

func RingCheck(as *require.Assertions, nodes []*LocalNode, counter bool) {
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
	as := require.New(t)

	n1 := NewLocalNode(devConfig(as))
	n1.Create()

	<-time.After(waitInterval)

	n1.Stop()

	<-time.After(waitInterval)

	RingCheck(as, []*LocalNode{n1}, true)
}

func TestJoin(t *testing.T) {
	as := require.New(t)

	n2 := NewLocalNode(devConfig(as))
	n2.Create()

	n1 := NewLocalNode(devConfig(as))
	as.Nil(n1.Join(n2))

	<-time.After(waitInterval)

	n1.Stop()
	n2.Stop()

	// skip counter clockwise check because we stopped first
	RingCheck(as, []*LocalNode{
		n1,
		n2,
	}, false)
}

func TestRandomNodes(t *testing.T) {
	as := require.New(t)

	num := 8
	nodes, done := makeRing(as, num)

	done()

	<-time.After(waitInterval)

	for i := 0; i < num; i++ {
		as.Equal(nodes[i].getSuccessor().ID(), nodes[i].fingers[1].n.Load().(*atomicVNode).Node.ID())
		fmt.Printf("%d: %s\n---\n", nodes[i].ID(), nodes[i].FingerTrace())
	}
}
