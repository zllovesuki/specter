package rpc

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/zllovesuki/specter/spec/protocol"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type MemoryPipe struct {
	otherR io.ReadCloser
	otherW io.WriteCloser
}

func (p *MemoryPipe) Read(b []byte) (int, error) {
	return p.otherR.Read(b)
}

func (p *MemoryPipe) Write(b []byte) (int, error) {
	return p.otherW.Write(b)
}

func (p *MemoryPipe) Close() error {
	p.otherR.Close()
	p.otherW.Close()
	return nil
}

func GetPipes() (io.ReadWriteCloser, io.ReadWriteCloser) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	c1 := &MemoryPipe{
		otherR: r2,
		otherW: w1,
	}

	c2 := &MemoryPipe{
		otherR: r1,
		otherW: w2,
	}

	return c1, c2
}

func TestRPC(t *testing.T) {
	as := assert.New(t)
	c1, c2 := GetPipes()

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

	callCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err = r2.Call(callCtx, &protocol.RPC_Request{})
	as.Error(err)
	as.ErrorContains(err, ErrNoHandler.Error())

	r2Resp, err := r1.Call(callCtx, &protocol.RPC_Request{})
	as.Nil(err)
	as.NotNil(r2Resp)
	as.NotNil(r2Resp.PingResponse)
}
