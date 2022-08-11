package chord

import (
	"runtime"

	"kon.nect.sh/specter/spec/chord"

	"github.com/CAFxX/atomic128"
	"github.com/zhangyunhao116/skipmap"
)

// nodeState is an extension of chord.State, but with 16-bytes storage
// as opposed to 8-bytes, because the addition of logical timestamp
// to track state change history.
type nodeState struct {
	state   atomic128.Uint128
	history *skipmap.Uint64Map[chord.State]
}

func NewNodeState(initial chord.State) *nodeState {
	s := &nodeState{
		state:   atomic128.Uint128{},
		history: skipmap.NewUint64[chord.State](),
	}
	atomic128.StoreUint128(&s.state, [2]uint64{
		0,                 // logical timestamp
		(uint64)(initial), // state at timestamp
	})
	s.history.Store(0, initial)
	return s
}

func (s *nodeState) Transition(expected chord.State, new chord.State) bool {
	curr := atomic128.LoadUint128(&s.state)
	prev := [2]uint64{curr[0], (uint64)(expected)}
	next := [2]uint64{curr[0] + 1, (uint64)(new)}
	if atomic128.CompareAndSwapUint128(&s.state, prev, next) {
		s.history.Store(next[0], new)
		return true
	}
	return false
}

func (s *nodeState) Set(val chord.State) {
	for {
		curr := atomic128.LoadUint128(&s.state)
		next := [2]uint64{curr[0] + 1, (uint64)(val)}
		if atomic128.CompareAndSwapUint128(&s.state, curr, next) {
			s.history.Store(next[0], val)
			break
		}
		runtime.Gosched()
	}
}

func (s *nodeState) Get() chord.State {
	return chord.State(atomic128.LoadUint128(&s.state)[1])
}

func (s *nodeState) History() []chord.State {
	h := make([]chord.State, 0)
	s.history.Range(func(_ uint64, state chord.State) bool {
		h = append(h, state)
		return true
	})
	return h
}
