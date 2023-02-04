// MIT License

// Copyright (c) 2022 Enterprize Software
// https://github.com/enterprizesoftware/rate-counter

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package ratecounter

import (
	"math"
	"sync"
	"time"

	"go.uber.org/atomic"
)

type Rate struct {
	lock        sync.RWMutex
	sampleCount *atomic.Uint64
	total       *atomic.Uint64
	count       *atomic.Uint64
	opened      *atomic.Time
	samples     []*sample
	interval    time.Duration
	observe     time.Duration
}

type sample struct {
	stored   time.Time
	count    uint64
	duration time.Duration
}

func New(interval time.Duration, observe time.Duration) *Rate {
	num := uint64(math.Ceil(float64(observe) / float64(interval)))
	samples := make([]*sample, num)
	for i := range samples {
		samples[i] = &sample{}
	}
	return &Rate{
		interval:    interval,
		observe:     observe,
		samples:     samples,
		sampleCount: atomic.NewUint64(0),
		total:       atomic.NewUint64(0),
		count:       atomic.NewUint64(0),
		opened:      atomic.NewTime(time.Now()),
	}
}

func (r *Rate) RatePerInterval() float64 {
	return float64(r.interval) * r.rate()
}

func (r *Rate) RatePer(interval time.Duration) float64 {
	return float64(interval) * r.rate()
}

func (r *Rate) rate() float64 {
	r.lock.RLock()
	defer r.lock.RUnlock()

	var c uint64
	var d time.Duration
	var num = min(len(r.samples), r.sampleCount.Load())
	var now = time.Now()

	for i := 0; i < num; i++ {
		if now.Sub(r.samples[i].stored) <= r.observe {
			c += r.samples[i].count
			d += r.samples[i].duration
		}
	}

	if d == 0 {
		return 0
	}

	return float64(c) / float64(d.Nanoseconds())
}

func (r *Rate) Total() uint64 {
	return r.total.Load()
}

func (r *Rate) Increment() {
	r.IncrementBy(1)
}

func (r *Rate) IncrementBy(i int) {
	r.count.Add(uint64(i))
	r.total.Add(uint64(i))

	r.lock.RLock()
	if time.Since(r.opened.Load()) > r.interval {
		r.lock.RUnlock()
		r.captureSample()
	} else {
		r.lock.RUnlock()
	}
}

func (r *Rate) captureSample() {
	r.lock.Lock()

	now := time.Now()
	sc := r.sampleCount.Load()

	index := int(sc+1) % len(r.samples)
	r.samples[index].count = r.count.Load()
	r.samples[index].duration = now.Sub(r.opened.Load())
	r.samples[index].stored = now

	r.sampleCount.Inc()

	r.opened.Store(now)
	r.count.Store(0)

	r.lock.Unlock()
}

func min(x int, y uint64) int {
	if uint64(x) < y {
		return x
	}
	return int(y)
}
