//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"context"
	"io"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

	"github.com/stretchr/testify/mock"
)

type RPC struct {
	mock.Mock
}

var _ rpc.RPC = (*RPC)(nil)

func (r *RPC) Call(ctx context.Context, node *protocol.Node, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	args := r.Called(ctx, node, req)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(*protocol.RPC_Response), e
}

func (r *RPC) HandleRequest(ctx context.Context, conn io.ReadWriter, handler rpc.RPCHandler) error {
	args := r.Called(ctx, conn, handler)
	return args.Error(0)
}
