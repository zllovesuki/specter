package spec

import (
	"context"

	"specter/spec/protocol"
)

type RPCHandshakeFunc func(RPC) error

type RPCHandler func(context.Context, *protocol.RequestReply) error

type RPC interface {
	Call(context.Context, *protocol.RequestReply) (*protocol.RequestReply, error)
	Close() error
}
