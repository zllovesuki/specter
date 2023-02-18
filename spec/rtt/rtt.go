package rtt

import (
	"fmt"
	"time"

	"kon.nect.sh/specter/spec/protocol"
)

type Recorder interface {
	Snapshot(key string, past time.Duration) *Statistics
	RecordLatency(key string, latency float64)
	RecordSent(key string)
	RecordLost(key string)
	Drop(key string)
}

type Statistics struct {
	Since             time.Time
	Until             time.Time
	Min               time.Duration
	Average           time.Duration
	Max               time.Duration
	StandardDeviation time.Duration
	Sent              uint64
	Lost              uint64
}

func (s *Statistics) String() string {
	if s == nil {
		return "(N/A)"
	}
	var lost float64
	if s.Sent > 0 {
		lost = float64(s.Lost) / float64(s.Sent)
	}
	return fmt.Sprintf("%d packets transmitted, %d received, %.0f%% packet loss\nmin/avg/max/mdev = %v/%v/%v/%v", s.Sent, s.Sent-s.Lost, lost, s.Min, s.Average, s.Max, s.StandardDeviation)
}

func MakeMeasurementKey(node *protocol.Node) string {
	qMapKey := node.GetAddress() + "/"
	if node.GetUnknown() {
		qMapKey = qMapKey + "-1"
	} else {
		qMapKey = qMapKey + "PHY"
	}
	return qMapKey
}
