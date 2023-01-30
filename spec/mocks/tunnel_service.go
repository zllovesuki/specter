//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"context"

	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
)

type TunnelService struct {
	mock.Mock
}

var _ protocol.TunnelService = (*TunnelService)(nil)

func (t *TunnelService) Ping(ctx context.Context, req *protocol.ClientPingRequest) (*protocol.ClientPingResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.ClientPingResponse), nil
}

func (t *TunnelService) RegisterIdentity(ctx context.Context, req *protocol.RegisterIdentityRequest) (*protocol.RegisterIdentityResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.RegisterIdentityResponse), nil
}

func (t *TunnelService) GetNodes(ctx context.Context, req *protocol.GetNodesRequest) (*protocol.GetNodesResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.GetNodesResponse), nil
}

func (t *TunnelService) GenerateHostname(ctx context.Context, req *protocol.GenerateHostnameRequest) (*protocol.GenerateHostnameResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.GenerateHostnameResponse), nil
}

func (t *TunnelService) PublishTunnel(ctx context.Context, req *protocol.PublishTunnelRequest) (*protocol.PublishTunnelResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.PublishTunnelResponse), nil
}
