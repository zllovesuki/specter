package chord

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func routineOnce(assert *assert.Assertions, node *LocalNode) {
	assert.Nil(node.Stablize())
	for i := 1; i < MaxFingerEntries; i++ {
		assert.Nil(node.FixFinger(i))
	}
	assert.Nil(node.CheckPredecessor())
}

func TestCreate(t *testing.T) {
	assert := assert.New(t)
	logger, err := zap.NewDevelopment()
	assert.Nil(err)

	n1 := NewLocalNode(1, logger)

	routineOnce(assert, n1)

	assert.Nil(n1.predecessor)
	assert.Equal(n1.successor, n1)
}

func TestJoin(t *testing.T) {
	assert := assert.New(t)
	logger, err := zap.NewDevelopment()
	assert.Nil(err)

	n1 := NewLocalNode(1, logger)
	n2 := NewLocalNode(10, logger)

	assert.Nil(n1.Join(n2))

	routineOnce(assert, n1)
	routineOnce(assert, n2)

	assert.Equal(n2.ID(), n1.successor.ID())
	assert.Equal(n1.ID(), n2.predecessor.ID())
}

func TestMultiNode(t *testing.T) {
	assert := assert.New(t)
	logger, err := zap.NewDevelopment()
	assert.Nil(err)

	n1 := NewLocalNode(1, logger)
	n2 := NewLocalNode(10, logger)
	n3 := NewLocalNode(100, logger)

	assert.Nil(n1.Join(n3))

	routineOnce(assert, n1)
	routineOnce(assert, n3)

	assert.Nil(n2.Join(n3))

	routineOnce(assert, n1)
	routineOnce(assert, n2)
	routineOnce(assert, n3)

	assert.Equal(n3.ID(), n2.successor.ID())
	assert.Equal(n2.ID(), n1.successor.ID())
	assert.Equal(n1.ID(), n2.predecessor.ID())
	assert.Equal(n2.ID(), n3.predecessor.ID())
}
