package overlay

import (
	"context"
	"math/rand"
	"time"

	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/rpc"

	"go.uber.org/zap"
)

func randomTimeRange(t time.Duration) time.Duration {
	return time.Duration(rand.Int63n(int64(t*2)-int64(t)) + int64(t))
}

func (t *QUIC) reaper(ctx context.Context) {
	timer := time.NewTimer(quicConfig.MaxIdleTimeout)

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
			t.qMap.Range(func(key string, value interface{}) bool {
				v := value.(*nodeConnection)
				if err := v.quic.SendMessage(alive); err != nil {
					candidate = append(candidate, v)
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
						r.(rpc.RPC).Close()
					}
				}
			}

			timer.Reset(randomTimeRange(quicConfig.MaxIdleTimeout / 2))
		}
	}
}
