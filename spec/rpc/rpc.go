package rpc

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"kon.nect.sh/specter/spec/protocol"

	pool "github.com/libp2p/go-buffer-pool"
)

const (
	// uint32
	LengthSize = 4
)

type RPCHandshakeFunc func(RPC) error

type RPCHandler func(context.Context, *protocol.RPC_Request) (*protocol.RPC_Response, error)

type RPC interface {
	Call(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error)
	Close() error
}

func Receive(stream io.Reader, rr VTMarshaler) error {
	sb := pool.Get(LengthSize)
	defer pool.Put(sb)

	n, err := io.ReadFull(stream, sb)
	if err != nil {
		return fmt.Errorf("reading RPC message buffer size: %w", err)
	}
	if n != LengthSize {
		return fmt.Errorf("expected %d bytes to be read but %d bytes was read", LengthSize, n)
	}

	ms := binary.BigEndian.Uint32(sb)

	mb := pool.Get(int(ms))
	defer pool.Put(mb)

	n, err = io.ReadFull(stream, mb)
	if err != nil {
		return fmt.Errorf("reading RPC message: %w", err)
	}
	if ms != uint32(n) {
		return fmt.Errorf("expected %d bytes to be read but %d bytes was read", ms, n)
	}

	return rr.UnmarshalVT(mb)
}

func Send(stream io.Writer, rr VTMarshaler) error {
	l := rr.SizeVT()
	mb := pool.Get(LengthSize + l)
	defer pool.Put(mb)

	binary.BigEndian.PutUint32(mb[0:LengthSize], uint32(l))

	_, err := rr.MarshalToSizedBufferVT(mb[LengthSize:])
	if err != nil {
		return fmt.Errorf("encoding outbound RPC message: %w", err)
	}

	n, err := stream.Write(mb)
	if err != nil {
		return fmt.Errorf("sending RPC message: %w", err)
	}
	if n != LengthSize+l {
		return fmt.Errorf("expected %d bytes sent but %d bytes was sent", l, n)
	}

	return nil
}
