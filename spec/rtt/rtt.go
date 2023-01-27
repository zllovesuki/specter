package rtt

import (
	"fmt"
	"strconv"
	"time"

	"kon.nect.sh/specter/spec/protocol"
)

type Recorder interface {
	Snapshot(key string, past time.Duration) *Statistics
	Record(key string, latency float64)
	Drop(key string)
}

type Statistics struct {
	Since             time.Time
	Until             time.Time
	Min               time.Duration
	Average           time.Duration
	Max               time.Duration
	StandardDeviation time.Duration
}

func (s *Statistics) String() string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("min/avg/max/mdev = %v/%v/%v/%v", s.Min, s.Average, s.Max, s.StandardDeviation)
}

func MakeMeasurementKey(node *protocol.Node) string {
	qMapKey := node.GetAddress() + "/"
	if node.GetUnknown() {
		qMapKey = qMapKey + "-1"
	} else {
		qMapKey = qMapKey + strconv.FormatUint(node.GetId(), 10)
	}
	return qMapKey
}
