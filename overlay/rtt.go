package overlay

import (
	"context"
	"encoding/binary"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rtt"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/util"

	"github.com/quic-go/quic-go"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap"
)

func (t *QUIC) sendRTTSyn(ctx context.Context, q quic.Connection, peer *protocol.Node) {
	l := t.Logger.With(zap.String("endpoint", q.RemoteAddr().String()), zap.Object("peer", peer))

	var (
		qKey                      = t.makeCachedKey(peer)
		mapper                    = skipmap.NewUint64[int64]()
		counter uint64            = 1
		rttBuf  protocol.Datagram = protocol.Datagram{
			Type: protocol.Datagram_RTT_SYN,
			Data: make([]byte, 8),
		}
		err error
		buf []byte
	)

	t.rttMap.Store(qKey, mapper)

	go t.handleRTTLost(ctx, q, peer)

	for {
		select {
		case <-ctx.Done():
			return
		case <-q.Context().Done():
			return
		default:
			binary.BigEndian.PutUint64(rttBuf.Data, counter)
			buf, err = rttBuf.MarshalVT()
			if err != nil {
				l.Error("error encoding rtt syn datagram to proto", zap.Error(err))
				continue
			}
			mapper.Store(counter, time.Now().UnixNano())
			if err = q.SendDatagram(buf); err != nil {
				l.Error("failed to send rtt syn", zap.Error(err))
				mapper.Delete(counter)
				continue
			}
			if t.RTTRecorder != nil {
				t.RTTRecorder.RecordSent(rtt.MakeMeasurementKey(peer))
			}
			counter++
			time.Sleep(util.RandomTimeRange(transport.RTTMeasureInterval))
		}
	}
}

func (t *QUIC) handleRTTAck(ctx context.Context) {
	var (
		l        *zap.Logger
		qKey     string
		received int64
		sent     int64
		counter  uint64
		ok       bool
		mapper   *skipmap.Uint64Map[int64]
		d        *transport.DatagramDelegate
	)

	for {
		select {
		case <-ctx.Done():
			t.Logger.Info("Exiting RTT ack goroutine")
			return
		case d = <-t.rttChan:
			l = t.Logger.With(zap.Object("peer", d.Identity))
			qKey = t.makeCachedKey(d.Identity)
			mapper, ok = t.rttMap.Load(qKey)
			if !ok {
				l.Warn("No mapping found for processing rtt ack")
				continue
			}
			if len(d.Buffer) != 8 {
				l.Warn("invalid length for counter timestamp", zap.Int("got", len(d.Buffer)))
				continue
			}
			received = time.Now().UnixNano()
			counter = binary.BigEndian.Uint64(d.Buffer)
			sent, ok = mapper.LoadAndDelete(counter)
			if !ok {
				l.Warn("no timestamp found for counter", zap.Uint64("counter", counter))
				continue
			}
			if t.RTTRecorder != nil {
				t.RTTRecorder.RecordLatency(rtt.MakeMeasurementKey(d.Identity), float64(received-sent))
			}
		}
	}
}

func (t *QUIC) handleRTTLost(ctx context.Context, q quic.Connection, peer *protocol.Node) {
	if t.RTTRecorder == nil {
		return
	}

	var (
		qKey   = t.makeCachedKey(peer)
		mapper *skipmap.Uint64Map[int64]
		ok     bool
	)

	mapper, ok = t.rttMap.Load(qKey)
	if !ok {
		t.Logger.Error("No mapper found for peer", zap.Object("peer", peer))
		return
	}

	ticker := time.NewTicker(transport.RTTMeasureInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-q.Context().Done():
			return
		case <-ticker.C:
			mapper.Range(func(counter uint64, sent int64) bool {
				if time.Since(time.Unix(0, sent)) > 2*transport.RTTMeasureInterval {
					t.RTTRecorder.RecordLost(rtt.MakeMeasurementKey(peer))
					mapper.Delete(counter)
				}
				return true
			})
		}
	}
}
