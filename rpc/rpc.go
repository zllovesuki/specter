package rpc

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

	pool "github.com/libp2p/go-buffer-pool"
	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

const (
	// uint64
	lengthSize = 8
)

var (
	ErrClosed      = fmt.Errorf("RPC channel already closed")
	ErrNoHandler   = fmt.Errorf("RPC channel has no request handler")
	ErrInvalidRing = fmt.Errorf("RPC calls to an invalid ring, expected m = %d", chord.MaxFingerEntries)
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
		if err := Receive(r.stream, rr); err != nil {
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

				if err := Send(r.stream, rr); err != nil {
					// r.logger.Error("RPC receiver send error", zap.Error(err))
					r.Close()
				}
			default:
			}
		}(rr)
	}
}

func (r *RPC) Close() error {
	if !r.closed.CAS(false, true) {
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

	if err := Send(r.stream, rr); err != nil {
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

func Receive(stream io.Reader, rr VTMarshaler) error {
	sb := pool.Get(lengthSize)
	defer pool.Put(sb)

	n, err := io.ReadFull(stream, sb)
	if err != nil {
		return fmt.Errorf("reading RPC message buffer size: %w", err)
	}
	if n != lengthSize {
		return fmt.Errorf("expected %d bytes to be read but %d bytes was read", lengthSize, n)
	}

	ms := binary.BigEndian.Uint64(sb)

	mb := pool.Get(int(ms))
	defer pool.Put(mb)

	n, err = io.ReadFull(stream, mb)
	if err != nil {
		return fmt.Errorf("reading RPC message: %w", err)
	}
	if ms != uint64(n) {
		return fmt.Errorf("expected %d bytes to be read but %d bytes was read", ms, n)
	}

	return rr.UnmarshalVT(mb)
}

func Send(stream io.Writer, rr VTMarshaler) error {
	buf, err := rr.MarshalVT()
	if err != nil {
		return fmt.Errorf("encoding outbound RPC message: %w", err)
	}

	l := len(buf)

	mb := pool.Get(lengthSize + l)
	defer pool.Put(mb)

	binary.BigEndian.PutUint64(mb[0:lengthSize], uint64(l))
	copy(mb[lengthSize:], buf)

	n, err := stream.Write(mb)
	if err != nil {
		return fmt.Errorf("sending RPC message: %w", err)
	}
	if n != lengthSize+l {
		return fmt.Errorf("expected %d bytes sent but %d bytes was sent", l, n)
	}

	return nil
}
