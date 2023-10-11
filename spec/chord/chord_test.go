package chord

import (
	"fmt"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/require"
)

// func TestHash(t *testing.T) {
// 	as := require.New(t)

// 	b := []byte("test key")
// 	as.Equal(wyhash.Sum64(MaxIdentitifer, b)%MaxIdentitifer, Hash(b))
// }

func TestModulo(t *testing.T) {
	var (
		as        = require.New(t)
		x  uint64 = 1 << 24
		y  uint64 = 1 << 28
	)

	as.Equal((x+y)%MaxIdentitifer, ModuloSum(x, y))
}

func TestRandom(t *testing.T) {
	as := require.New(t)

	t1 := Random()
	time.Sleep(time.Second)
	t2 := Random()

	as.NotEqual(t1, t2)
}

func TestBetweenExclusive(t *testing.T) {
	tables := []struct {
		low    uint64
		target uint64
		high   uint64
		result bool
	}{
		{
			low:    10,
			target: 20,
			high:   10,
			result: true,
		},
		{
			low:    10,
			target: 10,
			high:   20,
			result: false,
		},
		{
			low:    20,
			target: 10,
			high:   10,
			result: false,
		},
	}

	for _, table := range tables {
		t.Run(fmt.Sprintf("%d ∈ (%d, %d) == %v", table.target, table.low, table.high, table.result), func(t *testing.T) {
			as := require.New(t)
			as.Condition(func() (success bool) {
				return table.result == Between(table.low, table.target, table.high, false)
			})
		})
	}
}

type fakeNode struct {
	VNode
	identity *protocol.Node
}

func (f *fakeNode) ID() uint64 {
	return f.identity.GetId()
}

func (f *fakeNode) Identity() *protocol.Node {
	return f.identity
}

func TestSuccListNoDuplicateByID(t *testing.T) {
	as := require.New(t)

	nodes := []VNode{
		&fakeNode{nil, &protocol.Node{Id: 2}},
		&fakeNode{nil, &protocol.Node{Id: 2}},
		&fakeNode{nil, &protocol.Node{Id: 2}},
		&fakeNode{nil, &protocol.Node{Id: 3}},
		&fakeNode{nil, &protocol.Node{Id: 4}},
	}
	immediate := &fakeNode{nil, &protocol.Node{Id: 1}}

	list := MakeSuccListByID(immediate, nodes, 3)
	as.Len(list, 3)
	as.Equal(uint64(1), list[0].ID())
}

func TestSuccListNoDuplicateByAddress(t *testing.T) {
	as := require.New(t)

	nodes := []VNode{
		&fakeNode{nil, &protocol.Node{Address: "127.0.0.2"}},
		&fakeNode{nil, &protocol.Node{Address: "127.0.0.2"}},
		&fakeNode{nil, &protocol.Node{Address: "127.0.0.2"}},
		&fakeNode{nil, &protocol.Node{Address: "127.0.0.3"}},
		&fakeNode{nil, &protocol.Node{Address: "127.0.0.4"}},
	}
	immediate := &fakeNode{nil, &protocol.Node{Address: "127.0.0.1"}}

	list := MakeSuccListByAddress(immediate, nodes, 3)
	as.Len(list, 3)
	as.Equal("127.0.0.1", list[0].Identity().GetAddress())
}
