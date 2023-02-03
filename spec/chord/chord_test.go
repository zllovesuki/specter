package chord

import (
	"fmt"
	"testing"
	"time"

	"github.com/orisano/wyhash"
	"github.com/stretchr/testify/require"
)

func TestHash(t *testing.T) {
	as := require.New(t)

	b := []byte("test key")
	as.Equal(wyhash.Sum64(MaxIdentitifer, b)>>(64-MaxFingerEntries), Hash(b))
}

func TestModulo(t *testing.T) {
	var (
		as        = require.New(t)
		x  uint64 = 1 << 60
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
	id uint64
}

func (f *fakeNode) ID() uint64 {
	return f.id
}

func TestSuccListNoDuplicate(t *testing.T) {
	as := require.New(t)

	nodes := []VNode{
		&fakeNode{nil, 2},
		&fakeNode{nil, 2},
		&fakeNode{nil, 2},
		&fakeNode{nil, 3},
		&fakeNode{nil, 4},
	}
	immediate := &fakeNode{nil, 1}

	list := MakeSuccList(immediate, nodes, 3)
	as.Len(list, 3)
	as.Equal(uint64(1), list[0].ID())
}
