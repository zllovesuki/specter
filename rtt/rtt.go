package rtt

import (
	"sync"
	"time"

	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/util"

	"github.com/montanaflynn/stats"
	"github.com/zhangyunhao116/skipmap"
)

type point struct {
	time  time.Time
	value float64
}

type container struct {
	mu   sync.RWMutex
	data []point
	sent uint64
	lost uint64
}

type Instrumentation struct {
	measurement *skipmap.StringMap[*container]
	length      int
}

var _ rtt.Recorder = (*Instrumentation)(nil)

func NewInstrumentation(max int) *Instrumentation {
	return &Instrumentation{
		measurement: skipmap.NewString[*container](),
		length:      max,
	}
}

func (i Instrumentation) getContainer(key string) *container {
	c, _ := i.measurement.LoadOrStoreLazy(key, func() *container {
		return &container{
			data: make([]point, 0),
		}
	})
	return c
}

func (i *Instrumentation) RecordLatency(key string, value float64) {
	if value < 0 {
		return
	}

	c := i.getContainer(key)

	c.mu.Lock()
	if len(c.data) > i.length {
		c.data = c.data[1:]
	}
	c.data = append(c.data, point{
		time:  time.Now(),
		value: value,
	})
	c.mu.Unlock()
}

func (i *Instrumentation) RecordSent(key string) {
	c := i.getContainer(key)
	c.mu.Lock()
	c.sent++
	c.mu.Unlock()
}

func (i *Instrumentation) RecordLost(key string) {
	c := i.getContainer(key)
	c.mu.Lock()
	c.lost++
	c.mu.Unlock()
}

func (i *Instrumentation) Snapshot(key string, last time.Duration) *rtt.Statistics {
	c, ok := i.measurement.Load(key)
	if !ok {
		return nil
	}
	var (
		values = make([]float64, 0)
		since  time.Time
		until  time.Time
		sent   uint64
		lost   uint64
	)
	c.mu.RLock()
	for _, p := range c.data {
		if time.Since(p.time) <= last {
			if since.IsZero() {
				since = p.time
			}
			until = p.time
			values = append(values, p.value)
		}
	}
	sent = c.sent
	lost = c.lost
	c.mu.RUnlock()
	if len(values) < 1 {
		return nil
	}
	return &rtt.Statistics{
		Since:             since,
		Until:             until,
		Min:               time.Duration(util.Must(stats.Min(values))),
		Average:           time.Duration(util.Must(stats.Mean(values))),
		Max:               time.Duration(util.Must(stats.Max(values))),
		StandardDeviation: time.Duration(util.Must(stats.StandardDeviation(values))),
		Sent:              sent,
		Lost:              lost,
	}
}

func (i *Instrumentation) Drop(key string) {
	i.measurement.Delete(key)
}
