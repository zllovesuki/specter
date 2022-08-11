package overlay

import (
	"context"
	"math/rand"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/util"

	"go.uber.org/zap"
)

func (t *QUIC) reaper(ctx context.Context) {
	timer := time.NewTimer(util.RandomTimeRange(quicConfig.HandshakeIdleTimeout))

	d := &protocol.Datagram{
		Type: protocol.Datagram_ALIVE,
	}
	alive, err := d.MarshalVT()
	if err != nil {
		panic(err)
	}

	rand.Seed(time.Now().UnixNano())

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			candidate := make([]*nodeConnection, 0)
			t.qMap.Range(func(key string, value *nodeConnection) bool {
				if err := value.quic.SendMessage(alive); err != nil {
					candidate = append(candidate, value)
				}
				return true
			})

			if len(candidate) > 0 {
				for _, c := range candidate {
					k := makeQKey(c.peer)
					t.Logger.Debug("reaping cached QUIC connection to peer", zap.String("key", k))
					t.qMap.Delete(k)
					c.quic.CloseWithError(401, "Gone")
				}

				for _, c := range candidate {
					k := makeSKey(c.peer)
					t.Logger.Debug("reaping cached RPC channels to peer", zap.String("key", k))
					if r, ok := t.rpcMap.LoadAndDelete(k); ok {
						r.Close()
					}
				}
			}

			timer.Reset(util.RandomTimeRange(quicConfig.MaxIdleTimeout))
		}
	}
}
