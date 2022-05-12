package chord

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func RingCheck(as *assert.Assertions, nodes []*LocalNode) {
	if len(nodes) == 0 {
		return
	}
	for _, node := range nodes {
		as.NotNil(node.predecessor)
		as.NotNil(node.successor)
	}
	if len(nodes) == 1 {
		as.Equal(nodes[0].ID(), nodes[0].predecessor.ID())
		as.Equal(nodes[0].ID(), nodes[0].successor.ID())
		return
	}
	// counter clockwise
	for i := 0; i < len(nodes)-1; i++ {
		as.Equal(nodes[i].ID(), nodes[i+1].predecessor.ID())
	}
	as.Equal(nodes[len(nodes)-1].ID(), nodes[0].predecessor.ID())
	// clockwise
	for i := 0; i < len(nodes)-1; i++ {
		as.Equal(nodes[i+1].ID(), nodes[i].successor.ID())
	}
	as.Equal(nodes[0].ID(), nodes[len(nodes)-1].successor.ID())

	fmt.Printf("Ring: %s", nodes[0].Trace())
}

func TestCreate(t *testing.T) {
	as := assert.New(t)
	logger, err := zap.NewDevelopment()
	as.Nil(err)

	n1 := NewLocalNode(1, logger)
	n1.Create()
	n1.StartTasks()

	<-time.After(time.Second)

	RingCheck(as, []*LocalNode{n1})
}

func TestJoin(t *testing.T) {
	assert := assert.New(t)
	logger, err := zap.NewDevelopment()
	assert.Nil(err)

	n2 := NewLocalNode(2, logger)
	n2.Create()
	n2.StartTasks()

	n1 := NewLocalNode(1, logger)
	assert.Nil(n1.Join(n2))
	n1.StartTasks()

	<-time.After(time.Second)

	RingCheck(assert, []*LocalNode{
		n1,
		n2,
	})
}

func TestMultiNode(t *testing.T) {
	assert := assert.New(t)
	logger, err := zap.NewDevelopment()
	assert.Nil(err)

	n1 := NewLocalNode(1, logger)
	n2 := NewLocalNode(10, logger)
	n3 := NewLocalNode(100, logger)
	n4 := NewLocalNode(1000, logger)

	n4.Create()
	n4.StartTasks()

	assert.Nil(n2.Join(n4))
	n2.StartTasks()

	<-time.After(time.Second)

	RingCheck(assert, []*LocalNode{
		n2,
		n4,
	})

	assert.Nil(n3.Join(n4))
	n3.StartTasks()

	<-time.After(time.Second)

	RingCheck(assert, []*LocalNode{
		n2,
		n3,
		n4,
	})

	assert.Nil(n1.Join(n3))
	n1.StartTasks()

	<-time.After(time.Second)

	RingCheck(assert, []*LocalNode{
		n1,
		n2,
		n3,
		n4,
	})

	n5 := NewLocalNode(10000, logger)
	assert.Nil(n5.Join(n1))
	n5.StartTasks()

	<-time.After(time.Second)

	RingCheck(assert, []*LocalNode{
		n1,
		n2,
		n3,
		n4,
		n5,
	})
}
