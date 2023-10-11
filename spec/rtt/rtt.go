package rtt

import (
	"fmt"
	"time"

	"go.miragespace.co/specter/spec/protocol"
)

type Recorder interface {
	Snapshot(key string, past time.Duration) *Statistics
	RecordLatency(key string, latency float64)
	RecordSent(key string)
	RecordLost(key string)
	Drop(key string)
}

type Statistics struct {
	Since             time.Time     `json:"since"`
	Until             time.Time     `json:"until"`
	Min               time.Duration `json:"min"`
	Average           time.Duration `json:"average"`
	Max               time.Duration `json:"max"`
	StandardDeviation time.Duration `json:"mdev"`
	Sent              uint64        `json:"sent"`
	Lost              uint64        `json:"lost"`
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
