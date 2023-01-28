package rpc

import (
	"context"
	"fmt"
	"io"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"

	"go.uber.org/atomic"
	"go.uber.org/zap"
)

var _ rpc.RPC = (*RPC)(nil)

var (
	ErrClosed        = fmt.Errorf("rpc: channel already closed")
	ErrNoHandler     = fmt.Errorf("rpc: channel has no request handler")
	ErrInvalidRing   = fmt.Errorf("rpc: calls to an invalid ring, expected m = %d", chord.MaxFingerEntries)
	ErrExpectRequest = fmt.Errorf("rpc: unexpected RPC type; expecting Request")
	ErrExpectReply   = fmt.Errorf("rpc: unexpected RPC type; expecting Reply")
)

type rrContainer struct {
	resp *protocol.RPC_Response
	err  error
}
type rrChan chan rrContainer

type remoteError struct {
	msg string
}

func (r *remoteError) Error() string {
	return r.msg
}

type RPC struct {
	logger *zap.Logger

	ctx       context.Context
	transport transport.Transport

	closed  *atomic.Bool
	maxSize *atomic.Uint32
}

func defaultHandler(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	return nil, ErrNoHandler
}

func NewRPC(ctx context.Context, logger *zap.Logger, transport transport.Transport) *RPC {
	logger.Debug("Creating new RPC handler", zap.Bool("client", transport != nil))

	return &RPC{
		logger:    logger,
		ctx:       ctx,
		transport: transport,
		closed:    atomic.NewBool(false),
		maxSize:   atomic.NewUint32(0),
	}
}

func (r *RPC) LimitMessageSize(size uint32) {
	r.maxSize.Store(size)
}

func (r *RPC) receive(conn io.Reader, rr rpc.VTMarshaler) error {
	var (
		receiveErr error
		maxSize    uint32 = r.maxSize.Load()
	)
	if maxSize > 0 {
		receiveErr = rpc.BoundedReceive(conn, rr, maxSize)
	} else {
		receiveErr = rpc.Receive(conn, rr)
	}
	return receiveErr
}

func (r *RPC) HandleRequest(ctx context.Context, conn io.ReadWriter, handler rpc.RPCHandler) error {
	if handler == nil {
		handler = defaultHandler
	}

	rr := protocol.RPCFromVTPool()
	defer rr.ReturnToVTPool()

	if err := r.receive(conn, rr); err != nil {
		return err
	}

	if rr.GetType() != protocol.RPC_REQUEST {
		return ErrExpectRequest
	}

	if rr.GetRing() != chord.MaxFingerEntries {
		rr.Response = &protocol.RPC_Response{
			Error: []byte(ErrInvalidRing.Error()),
		}
		goto RESPOND
	}
	if resp, err := handler(ctx, rr.GetRequest()); err != nil {
		rr.Response = &protocol.RPC_Response{
			Error: []byte(err.Error()),
		}
	} else {
		rr.Response = resp
	}
RESPOND:
	rr.Request = nil
	rr.Type = protocol.RPC_REPLY

	return rpc.Send(conn, rr)
}

func (r *RPC) awaitResponse(respChan rrChan, conn io.Reader) {
	var (
		rr   = protocol.RPCFromVTPool()
		resp = rrContainer{}
	)
	defer rr.ReturnToVTPool()

	defer func() {
		respChan <- resp
	}()

	if err := r.receive(conn, rr); err != nil {
		resp.err = err
		return
	}

	if rr.GetType() != protocol.RPC_REPLY {
		resp.err = ErrExpectReply
		return
	}

	if rr.GetResponse().GetError() != nil {
		resp.err = &remoteError{string(rr.GetResponse().GetError())}
		return
	}

	resp.resp = rr.GetResponse()
}

func (r *RPC) Call(ctx context.Context, node *protocol.Node, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	if r.closed.Load() {
		return nil, ErrClosed
	}

	rr := protocol.RPCFromVTPool()
	defer rr.ReturnToVTPool()

	rr.Type = protocol.RPC_REQUEST
	rr.Ring = chord.MaxFingerEntries
	rr.Request = req

	conn, err := r.transport.DialStream(r.ctx, node, protocol.Stream_RPC)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := rpc.Send(conn, rr); err != nil {
		return nil, err
	}
	rC := make(rrChan, 1)

	go r.awaitResponse(rC, conn)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case rs := <-rC:
		if rs.resp == nil {
			return nil, chord.ErrorMapper(rs.err)
		}
		return rs.resp, nil
	}
}
