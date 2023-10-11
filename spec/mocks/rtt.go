//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"time"

	"go.miragespace.co/specter/spec/rtt"

	"github.com/stretchr/testify/mock"
)

type Measurement struct {
	mock.Mock
}

func (m *Measurement) Snapshot(key string, past time.Duration) *rtt.Statistics {
	args := m.Called(key, past)
	s := args.Get(0)
	if s == nil {
		return nil
	}
	return s.(*rtt.Statistics)
}

func (m *Measurement) RecordLatency(key string, latency float64) {
	m.Called(key, latency)
}

func (m *Measurement) RecordSent(key string) {
	m.Called(key)
}

func (m *Measurement) RecordLost(key string) {
	m.Called(key)
}

func (m *Measurement) Drop(key string) {
	m.Called(key)
}

var _ rtt.Recorder = (*Measurement)(nil)
