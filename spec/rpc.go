package spec

import (
	"context"

	"specter/spec/protocol"
)

type RPCHandshakeFunc func(RPC) error

type RPCHandler func(context.Context, *protocol.RPC_Request) (*protocol.RPC_Response, error)

type RPC interface {
	Call(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error)
	Close() error
}
