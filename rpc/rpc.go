package rpc

import (
	"context"
	"fmt"
	"io"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

var (
	ErrClosed      = fmt.Errorf("rpc: channel already closed")
	ErrNoHandler   = fmt.Errorf("rpc: channel has no request handler")
	ErrInvalidRing = fmt.Errorf("rpc: calls to an invalid ring, expected m = %d", chord.MaxFingerEntries)
)

var _ rpc.RPC = (*RPC)(nil)

type rrContainer struct {
	rr *protocol.RPC_Response
	e  error
}

type remoteError struct {
	msg string
}

func (r *remoteError) Error() string {
	return r.msg
}

type rrChan chan rrContainer

type RPC struct {
	logger *zap.Logger

	stream io.ReadWriteCloser
	num    *atomic.Uint64

	rMap *skipmap.Uint64Map[chan rrContainer]

	closed *atomic.Bool

	handler rpc.RPCHandler
}

func defaultHandler(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	return nil, ErrNoHandler
}

func NewRPC(logger *zap.Logger, stream io.ReadWriteCloser, handler rpc.RPCHandler) *RPC {
	logger.Debug("Creating new RPC handler")
	if handler == nil {
		handler = defaultHandler
	}
	return &RPC{
		logger:  logger,
		stream:  stream,
		num:     atomic.NewUint64(0),
		rMap:    skipmap.NewUint64[chan rrContainer](),
		handler: handler,
		closed:  atomic.NewBool(false),
	}
}

func (r *RPC) Start(ctx context.Context) {
	for {
		rr := protocol.RPCFromVTPool()
		if err := rpc.Receive(r.stream, rr); err != nil {
			// r.logger.Error("RPC receive read error", zap.Error(err))
			r.Close()
			return
		}

		go func(rr *protocol.RPC) {
			defer rr.ReturnToVTPool()

			switch rr.GetType() {
			case protocol.RPC_REPLY:
				rC, ok := r.rMap.LoadAndDelete(rr.GetReqNum())
				if !ok {
					return
				}
				c := rrContainer{}
				if rr.GetResponse().GetError() != nil {
					c.e = &remoteError{string(rr.GetResponse().GetError())}
				} else {
					c.rr = rr.GetResponse()
				}
				select {
				case rC <- c:
				default:
				}

			case protocol.RPC_REQUEST:
				if rr.GetRing() != chord.MaxFingerEntries {
					rr.Response = &protocol.RPC_Response{
						Error: []byte(ErrInvalidRing.Error()),
					}
					goto RESPOND
				}
				if resp, err := r.handler(ctx, rr.GetRequest()); err != nil {
					rr.Response = &protocol.RPC_Response{
						Error: []byte(err.Error()),
					}
				} else {
					rr.Response = resp
				}
			RESPOND:
				rr.Request = nil
				rr.Type = protocol.RPC_REPLY

				if err := rpc.Send(r.stream, rr); err != nil {
					// r.logger.Error("RPC receiver send error", zap.Error(err))
					r.Close()
				}
			default:
			}
		}(rr)
	}
}

func (r *RPC) Close() error {
	if !r.closed.CompareAndSwap(false, true) {
		return nil
	}
	r.logger.Debug("Closing RPC channel")
	return r.stream.Close()
}

func (r *RPC) Call(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	if r.closed.Load() {
		return nil, ErrClosed
	}

	rNum := r.num.Inc()
	rC := make(rrChan, 1)

	rr := protocol.RPCFromVTPool()
	defer rr.ReturnToVTPool()

	rr.Type = protocol.RPC_REQUEST
	rr.ReqNum = rNum
	rr.Ring = chord.MaxFingerEntries
	rr.Request = req

	r.rMap.Store(rNum, rC)
	defer r.rMap.Delete(rNum)

	if err := rpc.Send(r.stream, rr); err != nil {
		r.Close()
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case rs := <-rC:
		if rs.rr == nil {
			return nil, rs.e
		}
		return rs.rr, nil
	}
}
