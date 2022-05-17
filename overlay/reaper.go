package overlay

import (
	"context"
	"math/rand"
	"time"

	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/rpc"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func randomTimeRange(t time.Duration) time.Duration {
	return time.Duration(rand.Int63n(int64(t*2)-int64(t)) + int64(t))
}

func (t *QUIC) printCached() {
	ep := make([]string, 0)
	t.qMap.Range(func(key string, value interface{}) bool {
		ep = append(ep, key)
		return true
	})
	t.Logger.Debug("Cached QUIC endpoints", zap.Strings("keys", ep))
}

func (t *QUIC) reaper(ctx context.Context) {
	timer := time.NewTimer(quicConfig.MaxIdleTimeout)

	alive, err := proto.Marshal(&protocol.Datagram{
		Type: protocol.Datagram_ALIVE,
	})
	if err != nil {
		panic(err)
	}

	rand.Seed(time.Now().UnixNano())

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			// t.printCached()
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
					go t.Delegate.TransportDestroyed(c.peer)
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
