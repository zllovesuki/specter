package overlay

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"specter/spec/protocol"

	"github.com/lucas-clemente/quic-go"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type rrContainer struct {
	rr *protocol.RequestReply
	e  string
}

type rrChan chan rrContainer

type rpcHandler func(context.Context, *protocol.RequestReply) error

type RPC struct {
	logger *zap.Logger

	stream quic.Stream
	num    *atomic.Uint64

	// TODO: candidate for 1.18 generics
	rMap sync.Map

	last *atomic.Time

	handler rpcHandler
}

func NewRPC(logger *zap.Logger, stream quic.Stream, handler rpcHandler) *RPC {
	return &RPC{
		logger:  logger,
		stream:  stream,
		num:     atomic.NewUint64(0),
		handler: handler,
		last:    atomic.NewTime(time.Now()),
	}
}

func (r *RPC) Start(ctx context.Context) {
	for {
		rr := &protocol.RequestReply{}
		if err := receiveRPC(r.stream, rr); err != nil {
			r.logger.Error("RPC receive error", zap.Error(err))
			r.Close()
			return
		}
		r.last.Store(time.Now())

		go func(rr *protocol.RequestReply) {
			switch rr.GetType() {
			case protocol.RequestReply_REPLY:
				rC, ok := r.rMap.LoadAndDelete(rr.GetReqNum())
				if !ok {
					return
				}
				c := rrContainer{}
				if rr.Errror != nil {
					c.e = string(rr.Errror)
				} else {
					c.rr = rr
				}
				rC.(rrChan) <- c

			case protocol.RequestReply_REQUEST:
				if err := r.handler(ctx, rr); err != nil {
					rr.Errror = []byte(err.Error())
				}
				if err := sendRPC(r.stream, rr); err != nil {
					r.logger.Debug("RPC receiver send error", zap.Error(err))
					r.Close()
				}
				r.last.Store(time.Now())
			default:
			}
		}(rr)
	}
}

func (r *RPC) Close() error {
	r.last.Store(time.Time{})
	return r.stream.Close()
}

func (r *RPC) Call(rr *protocol.RequestReply) (*protocol.RequestReply, error) {
	if r.last.Load().IsZero() {
		return nil, fmt.Errorf("RPC channel already closed")
	}
	rNum := r.num.Inc()
	rC := make(rrChan)
	rr.ReqNum = rNum
	r.rMap.Store(rNum, rC)
	if err := sendRPC(r.stream, rr); err != nil {
		r.logger.Debug("RPC caller send error", zap.Error(err))
		r.rMap.Delete(rNum)
		r.Close()
		return nil, err
	}
	r.last.Store(time.Now())

	select {
	case <-time.After(time.Second):
		return nil, fmt.Errorf("RPC Call timeout after 1 second")
	case rs := <-rC:
		if rs.rr == nil {
			return nil, fmt.Errorf("remote RPC error: %s", rs.e)
		}
		return rs.rr, nil
	}
}

func receiveRPC(stream quic.Stream, rr proto.Message) error {
	sb := make([]byte, 8)
	n, err := io.ReadFull(stream, sb)
	if err != nil {
		return fmt.Errorf("reading RPC message buffer size: %w", err)
	}
	if n != 8 {
		return fmt.Errorf("expected %d bytes to be read but %d bytes was read", 8, n)
	}

	ms := binary.BigEndian.Uint64(sb)
	mb := make([]byte, ms)
	n, err = io.ReadFull(stream, mb)
	if err != nil {
		return fmt.Errorf("reading RPC message: %w", err)
	}
	if ms != uint64(n) {
		return fmt.Errorf("expected %d bytes to be read but %d bytes was read", ms, n)
	}

	return proto.Unmarshal(mb, rr)
}

func sendRPC(stream quic.Stream, rr proto.Message) error {
	buf, err := proto.Marshal(rr)
	if err != nil {
		return fmt.Errorf("encoding outbound RPC message: %w", err)
	}

	l := len(buf)
	mb := make([]byte, 8+l)
	binary.BigEndian.PutUint64(mb[0:8], uint64(l))
	copy(mb[8:], buf)

	n, err := stream.Write(mb)
	if err != nil {
		return fmt.Errorf("sending RPC message: %w", err)
	}
	if n != 8+l {
		return fmt.Errorf("expected %d bytes sent but %d bytes was sent", l, n)
	}

	return nil
}
