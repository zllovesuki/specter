package overlay

import (
	"context"
	"encoding/binary"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rtt"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util"

	"github.com/quic-go/quic-go"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap"
)

func (t *QUIC) sendRTTSyn(ctx context.Context, q quic.Connection, peer *protocol.Node) {
	if peer.GetId() == t.Endpoint.GetId() {
		t.Logger.Debug("skipping rtt measurement with ourselves")
		return
	}

	l := t.Logger.With(zap.String("endpoint", q.RemoteAddr().String()), zap.Object("peer", peer))

	var (
		qKey                      = makeCachedKey(peer)
		mapper                    = skipmap.NewUint64[int64]()
		counter uint64            = 1
		rttBuf  protocol.Datagram = protocol.Datagram{
			Type: protocol.Datagram_RTT_SYN,
		}
		err        error
		buf        []byte
		counterBuf [8]byte
	)

	t.rttMap.Store(qKey, mapper)

	for {
		select {
		case <-ctx.Done():
			return
		case <-q.Context().Done():
			return
		default:
			binary.BigEndian.PutUint64(counterBuf[:], counter)
			rttBuf.Data = counterBuf[:]
			buf, err = rttBuf.MarshalVT()
			if err != nil {
				l.Error("error encoding rtt syn datagram to proto", zap.Error(err))
				continue
			}
			mapper.Store(counter, time.Now().UnixNano())
			if err = q.SendMessage(buf); err != nil {
				l.Error("failed to send rtt syn", zap.Error(err))
				mapper.Delete(counter)
				continue
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
			return
		case d = <-t.rttChan:
			l = t.Logger.With(zap.Object("peer", d.Identity))
			qKey = makeCachedKey(d.Identity)
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
				t.RTTRecorder.Record(rtt.MakeMeasurementKey(d.Identity), float64(received-sent))
			}
		}
	}
}
