package rpc

import (
	"context"
	"net"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRPC(t *testing.T) {
	as := require.New(t)
	c1, c2 := net.Pipe()

	logger, err := zap.NewDevelopment()
	as.Nil(err)

	resp := &protocol.RPC_Response{
		PingResponse: &protocol.PingResponse{},
	}
	r2Handler := func(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
		return resp, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r1 := NewRPC(logger, c1, nil)
	go r1.Start(ctx)
	defer r1.Close()

	r2 := NewRPC(logger, c2, r2Handler)
	go r2.Start(ctx)
	defer r2.Close()

	callCtx, callCancel := context.WithTimeout(ctx, time.Second*3)
	defer callCancel()

	_, err = r2.Call(callCtx, &protocol.RPC_Request{})
	as.Error(err)
	as.ErrorContains(err, ErrNoHandler.Error())

	r2Resp, err := r1.Call(callCtx, &protocol.RPC_Request{})
	as.Nil(err)
	as.NotNil(r2Resp)
	as.NotNil(r2Resp.PingResponse)
}
