package chord

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComaprison(t *testing.T) {
	x := rand.Uint64()
	y := rand.Uint64()

	as := assert.New(t)

	as.True(Between(x, y, x, false))
	as.False(Between(x, x, y, false))
	as.False(Between(y, x, x, false))
}
