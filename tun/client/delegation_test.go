package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
)

func TestDelegationCalls(t *testing.T) {
	c, m := newOutcomeTestClient(t, nil)
	c.connections.Store("z-second.example.com", &protocol.Node{Address: "z-second.example.com"})
	ambiguous := twirp.Unavailable.Error("write result unknown")
	m.TunnelService.On("MintDelegation", mock.Anything, mock.Anything).Return(nil, ambiguous).Once()
	_, err := c.MintDelegation(t.Context(), "test", time.Time{})
	require.ErrorIs(t, err, ErrMintAmbiguous)
	expected := err.Error()
	m.TunnelService.AssertNumberOfCalls(t, "MintDelegation", 1)
	matches := func(address string) any {
		return mock.MatchedBy(func(ctx context.Context) bool { return rpc.GetNode(ctx).GetAddress() == address })
	}
	response := &protocol.MintDelegationResponse{
		Grant: &protocol.DelegationGrant{
			Id:       strings.Repeat("a", 64),
			Hostname: "test",
		},
		Token: "tg1_secret",
	}
	m.TunnelService.On("MintDelegation", matches("gateway.example.com"), mock.Anything).Return(nil, twirp.BadRoute.Error("unsupported")).Once()
	m.TunnelService.On("MintDelegation", matches("z-second.example.com"), mock.Anything).Return(response, nil).Once()
	r := httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"hostname":"test"}`))
	w := httptest.NewRecorder()
	c.localHandler().ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Contains(t, w.Body.String(), response.Token)
	m.TunnelService.On("MintDelegation", mock.Anything, mock.Anything).Return(nil, ambiguous).Once()
	w = httptest.NewRecorder()
	c.localHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"hostname":"test"}`)))
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), expected)
	m.TunnelService.On("ListDelegations", matches("gateway.example.com"), mock.Anything).Return(nil, errors.New("temporary failure")).Once()
	m.TunnelService.On("ListDelegations", matches("z-second.example.com"), mock.Anything).Return(&protocol.ListDelegationsResponse{Grants: []*protocol.DelegationGrant{response.Grant}}, nil).Once()
	w = httptest.NewRecorder()
	c.localHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tokens", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "tg1_")
	require.Contains(t, w.Body.String(), response.Grant.Id)
	m.TunnelService.On("RevokeDelegation", mock.Anything, mock.Anything).Return(nil, twirp.NotFound.Error("grant is not owned by this client")).Once()
	w = httptest.NewRecorder()
	c.localHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/tokens/"+response.Grant.Id+"/revoke", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
	m.TunnelService.On("MintDelegation", mock.Anything, mock.Anything).Return(nil, twirp.ResourceExhausted.Error("delegation limit reached")).Once()
	w = httptest.NewRecorder()
	c.localHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/tokens", strings.NewReader(`{"hostname":"test"}`)))
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.NotContains(t, w.Body.String(), "ambiguous")
}
