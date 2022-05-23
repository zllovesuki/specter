package mocks

import (
	"context"

	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/rpc"

	"github.com/stretchr/testify/mock"
)

type RPC struct {
	mock.Mock
}

var _ rpc.RPC = (*RPC)(nil)

func (r *RPC) Call(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
	args := r.Called(ctx, req)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(*protocol.RPC_Response), e
}

func (r *RPC) Close() error {
	args := r.Called()
	return args.Error(0)
}
