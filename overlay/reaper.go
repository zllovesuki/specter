package overlay

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"specter/spec/protocol"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func randomTimeRange(t time.Duration) time.Duration {
	return time.Duration(rand.Int63n(int64(t*2)-int64(t)) + int64(t))
}

func rpcPrefixSearch(m map[string]*RPC, prefix string) []string {
	r := make([]string, 0)
	for k := range m {
		if strings.HasPrefix(k, prefix) {
			r = append(r, k)
		}
	}
	return r
}

func (t *Transport) reaper(ctx context.Context) {
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
			candidate := make([]*nodeConnection, 0)
			ep := make([]string, 0)
			t.qMu.RLock()
			for addr, v := range t.qMap {
				ep = append(ep, addr)
				if err := v.quic.SendMessage(alive); err != nil {
					candidate = append(candidate, v)
				}
			}
			t.qMu.RUnlock()
			t.logger.Debug("cached QUIC endpoints", zap.Strings("keys", ep))

			if len(candidate) > 0 {
				t.qMu.Lock()
				for _, c := range candidate {
					k := makeKey(c.peer)
					t.logger.Debug("reaping cached QUIC connection to peer", zap.String("key", k))
					delete(t.qMap, k)
					c.quic.CloseWithError(401, "Gone")
				}
				t.qMu.Unlock()

				t.rpcMu.Lock()
				for _, c := range candidate {
					p := rpcPrefixSearch(t.rpcMap, c.peer.GetAddress())
					t.logger.Debug("reaping cached RPC channels to peer", zap.Strings("key", p))
					for _, r := range p {
						t.rpcMap[r].Close()
						delete(t.rpcMap, r)
					}
				}
				t.rpcMu.Unlock()
			}

			timer.Reset(randomTimeRange(quicConfig.MaxIdleTimeout / 2))
		}
	}
}
