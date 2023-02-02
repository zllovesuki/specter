package chord

import (
	"runtime"
	"sync/atomic"

	"kon.nect.sh/specter/spec/chord"

	"github.com/zhangyunhao116/skipmap"
)

type nodeState struct {
	state   atomic.Uint64
	history *skipmap.Uint64Map[chord.State]
}

func newNodeState(initial chord.State) *nodeState {
	s := &nodeState{
		state:   atomic.Uint64{},
		history: skipmap.NewUint64[chord.State](),
	}
	var index uint64 = 0
	var state uint64 = (uint64)(initial)
	s.state.Store((index << 4) | state)
	s.history.Store(0, initial)
	return s
}

func (s *nodeState) Transition(exp chord.State, nxt chord.State) (chord.State, bool) {
	curr := s.state.Load()
	currIndex := curr >> 4
	prev := (currIndex << 4) | (uint64)(exp)
	nextIndex := currIndex + 1
	next := (nextIndex << 4) | (uint64)(nxt)
	if s.state.CompareAndSwap(prev, next) {
		s.history.Store(nextIndex, nxt)
		return nxt, true
	}
	return chord.State(curr & 0b1111), false
}

func (s *nodeState) Set(val chord.State) {
	for {
		if _, ok := s.Transition(s.Get(), val); ok {
			break
		}
		runtime.Gosched()
	}
}

func (s *nodeState) Get() chord.State {
	return chord.State(s.state.Load() & 0b1111)
}

func (s *nodeState) History() []chord.State {
	h := make([]chord.State, 0)
	s.history.Range(func(_ uint64, state chord.State) bool {
		h = append(h, state)
		return true
	})
	return h
}
