//go:build !no_mocks

package mocks

import (
	"context"

	"go.miragespace.co/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
)

type TunnelService struct {
	mock.Mock
	Keyless *KeylessService
}

var _ protocol.TunnelService = (*TunnelService)(nil)

func (t *TunnelService) OpenEphemeralSession(ctx context.Context, req *protocol.OpenEphemeralSessionRequest) (*protocol.OpenSessionResponse, error) {
	args := t.Called(ctx, req)
	if err := args.Error(1); err != nil {
		return nil, err
	}
	return args.Get(0).(*protocol.OpenSessionResponse), nil
}

func (t *TunnelService) OpenDelegatedSession(ctx context.Context, req *protocol.OpenDelegatedSessionRequest) (*protocol.OpenSessionResponse, error) {
	args := t.Called(ctx, req)
	if err := args.Error(1); err != nil {
		return nil, err
	}
	return args.Get(0).(*protocol.OpenSessionResponse), nil
}

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

func (t *TunnelService) RegisteredHostnames(ctx context.Context, req *protocol.RegisteredHostnamesRequest) (*protocol.RegisteredHostnamesResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.RegisteredHostnamesResponse), nil
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

func (t *TunnelService) UnpublishTunnel(ctx context.Context, req *protocol.UnpublishTunnelRequest) (*protocol.UnpublishTunnelResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.UnpublishTunnelResponse), nil
}

func (t *TunnelService) ReleaseTunnel(ctx context.Context, req *protocol.ReleaseTunnelRequest) (*protocol.ReleaseTunnelResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.ReleaseTunnelResponse), nil
}

func (t *TunnelService) AcmeInstruction(ctx context.Context, req *protocol.InstructionRequest) (*protocol.InstructionResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.InstructionResponse), nil
}

func (t *TunnelService) AcmeValidate(ctx context.Context, req *protocol.ValidateRequest) (*protocol.ValidateResponse, error) {
	args := t.Called(ctx, req)
	r := args.Get(0)
	e := args.Error(1)
	if e != nil {
		return nil, e
	}
	return r.(*protocol.ValidateResponse), nil
}

func (t *TunnelService) MintDelegation(ctx context.Context, req *protocol.MintDelegationRequest) (*protocol.MintDelegationResponse, error) {
	args := t.Called(ctx, req)
	if err := args.Error(1); err != nil {
		return nil, err
	}
	return args.Get(0).(*protocol.MintDelegationResponse), nil
}

func (t *TunnelService) ListDelegations(ctx context.Context, req *protocol.ListDelegationsRequest) (*protocol.ListDelegationsResponse, error) {
	args := t.Called(ctx, req)
	if err := args.Error(1); err != nil {
		return nil, err
	}
	return args.Get(0).(*protocol.ListDelegationsResponse), nil
}

func (t *TunnelService) RevokeDelegation(ctx context.Context, req *protocol.RevokeDelegationRequest) (*protocol.RevokeDelegationResponse, error) {
	args := t.Called(ctx, req)
	if err := args.Error(1); err != nil {
		return nil, err
	}
	return args.Get(0).(*protocol.RevokeDelegationResponse), nil
}
