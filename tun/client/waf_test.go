package client

import (
	"context"
	"testing"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
)

func TestHostWAFClientServerFallbackAndUncertainSave(t *testing.T) {
	c, service := newOutcomeTestClient(t, nil)
	c.connections.Store("z-second.example.com", &protocol.Node{Address: "z-second.example.com"})
	at := func(address string) any {
		return mock.MatchedBy(func(ctx context.Context) bool { return rpc.GetNode(ctx).GetAddress() == address })
	}
	response := &protocol.HostWAFResponse{Hostname: "test", Policy: &protocol.WAFPolicy{Mode: protocol.WAFPolicy_INHERIT}}
	service.TunnelService.On("GetHostWAF", at("gateway.example.com"), mock.Anything).Return(nil, twirp.BadRoute.Error("old server")).Once()
	service.TunnelService.On("GetHostWAF", at("z-second.example.com"), mock.Anything).Return(response, nil).Once()
	_, err := c.GetHostWAF(t.Context(), "test")
	require.NoError(t, err)

	uncertain := twirp.Unavailable.Error("save result is uncertain")
	service.TunnelService.On("SetHostWAF", at("gateway.example.com"), mock.Anything).Return(nil, uncertain).Once()
	_, err = c.SetHostWAF(t.Context(), "test", "original-version", &protocol.WAFPolicy{Mode: protocol.WAFPolicy_OFF})
	require.ErrorIs(t, err, uncertain)
	service.TunnelService.AssertNumberOfCalls(t, "SetHostWAF", 1)
}
