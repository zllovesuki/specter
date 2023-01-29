//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"context"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"

	"github.com/stretchr/testify/mock"
)

type ChordClient struct {
	mock.Mock
}

func (c *ChordClient) Identity(ctx context.Context, req *protocol.IdentityRequest) (*protocol.IdentityResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.IdentityResponse), nil
}

func (c *ChordClient) Ping(ctx context.Context, req *protocol.PingRequest) (*protocol.PingResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.PingResponse), nil
}

func (c *ChordClient) Notify(ctx context.Context, req *protocol.NotifyRequest) (*protocol.NotifyResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.NotifyResponse), nil
}

func (c *ChordClient) FindSuccessor(ctx context.Context, req *protocol.FindSuccessorRequest) (*protocol.FindSuccessorResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.FindSuccessorResponse), nil
}

func (c *ChordClient) GetSuccessors(ctx context.Context, req *protocol.GetSuccessorsRequest) (*protocol.GetSuccessorsResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.GetSuccessorsResponse), nil
}

func (c *ChordClient) GetPredecessor(ctx context.Context, req *protocol.GetPredecessorRequest) (*protocol.GetPredecessorResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.GetPredecessorResponse), nil
}

func (c *ChordClient) RequestToJoin(ctx context.Context, req *protocol.RequestToJoinRequest) (*protocol.RequestToJoinResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.RequestToJoinResponse), nil
}

func (c *ChordClient) FinishJoin(ctx context.Context, req *protocol.MembershipConclusionRequest) (*protocol.MembershipConclusionResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.MembershipConclusionResponse), nil
}

func (c *ChordClient) RequestToLeave(ctx context.Context, req *protocol.RequestToLeaveRequest) (*protocol.RequestToLeaveResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.RequestToLeaveResponse), nil
}

func (c *ChordClient) FinishLeave(ctx context.Context, req *protocol.MembershipConclusionRequest) (*protocol.MembershipConclusionResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.MembershipConclusionResponse), nil
}

func (c *ChordClient) Put(ctx context.Context, req *protocol.SimpleRequest) (*protocol.SimpleResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.SimpleResponse), nil
}

func (c *ChordClient) Get(ctx context.Context, req *protocol.SimpleRequest) (*protocol.SimpleResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.SimpleResponse), nil
}

func (c *ChordClient) Delete(ctx context.Context, req *protocol.SimpleRequest) (*protocol.SimpleResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.SimpleResponse), nil
}

func (c *ChordClient) Append(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.PrefixResponse), nil
}

func (c *ChordClient) List(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.PrefixResponse), nil
}

func (c *ChordClient) Contains(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.PrefixResponse), nil
}

func (c *ChordClient) Remove(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.PrefixResponse), nil
}

func (c *ChordClient) Acquire(ctx context.Context, req *protocol.LeaseRequest) (*protocol.LeaseResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.LeaseResponse), nil
}

func (c *ChordClient) Renew(ctx context.Context, req *protocol.LeaseRequest) (*protocol.LeaseResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.LeaseResponse), nil
}

func (c *ChordClient) Release(ctx context.Context, req *protocol.LeaseRequest) (*protocol.LeaseResponse, error) {
	args := c.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.LeaseResponse), nil
}

func (c *ChordClient) Import(_ context.Context, _ *protocol.ImportRequest) (*protocol.ImportResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *ChordClient) RatePer(interval time.Duration) float64 {
	return 1.0
}

var _ rpc.ChordClient = (*ChordClient)(nil)
