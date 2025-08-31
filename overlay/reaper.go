package overlay

import (
	"context"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/util"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

func (t *QUIC) reapPeer(q *quic.Conn, peer *protocol.Node) {
	qKey := t.makeCachedKey(peer)
	unlock := t.cachedMutex.Lock(qKey)
	defer unlock()

	t.Logger.Debug("reaping cached QUIC connection to peer", zap.String("key", qKey))
	cached, loaded := t.cachedConnections.LoadAndDelete(qKey)
	if loaded {
		cached.quic.CloseWithError(401, "Gone")
	}
	q.CloseWithError(401, "Gone")

	t.rttMap.Delete(qKey)
	if t.RTTRecorder != nil {
		t.RTTRecorder.Drop(rtt.MakeMeasurementKey(peer))
	}
}

// TODO: investigate if reaper is now deprecated
func (t *QUIC) reaper(ctx context.Context) {
	timer := time.NewTimer(util.RandomTimeRange(quicConfig.HandshakeIdleTimeout))

	d := &protocol.Datagram{
		Type: protocol.Datagram_ALIVE,
	}
	alive, err := d.MarshalVT()
	if err != nil {
		panic(err)
	}

	for {
		select {
		case <-ctx.Done():
			t.Logger.Info("Exiting reaper goroutine")
			return
		case <-timer.C:
			candidate := make([]*nodeConnection, 0)
			t.cachedConnections.Range(func(key string, value *nodeConnection) bool {
				if err := value.quic.SendDatagram(alive); err != nil {
					candidate = append(candidate, value)
				}
				return true
			})

			if len(candidate) > 0 {
				for _, c := range candidate {
					t.reapPeer(c.quic, c.peer)
				}
			}

			timer.Reset(util.RandomTimeRange(quicConfig.MaxIdleTimeout))
		}
	}
}
