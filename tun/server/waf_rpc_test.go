package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"
	"go.miragespace.co/specter/waf"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
)

func TestHostWAFPolicyOwnershipAndVersion(t *testing.T) {
	s, n, _, cert := sessionFixture(t)
	ctx, _ := sessionContext(t, cert)
	owner, _, err := extractAuthenticated(ctx)
	require.NoError(t, err)
	hostname := "owned-host"
	key := []byte(tun.HostWAFKey(hostname))
	prefix := []byte(tun.ClientHostnamesPrefix(owner))
	lease := []byte(tun.ClientLeaseKey(owner))
	n.On("PrefixContains", mock.Anything, prefix, []byte(hostname)).Return(true, nil).Times(4)
	n.On("Acquire", mock.Anything, lease, mock.Anything).Return(uint64(1), nil).Twice()
	n.On("Release", mock.Anything, lease, uint64(1)).Return(nil).Twice()
	n.On("Get", mock.Anything, key).Return(nil, nil).Twice()

	initial, err := s.GetHostWAF(ctx, &protocol.GetHostWAFRequest{Hostname: hostname})
	require.NoError(t, err)
	require.Empty(t, initial.Version)
	require.Equal(t, protocol.WAFPolicy_INHERIT, initial.Policy.Mode)
	var saved []byte
	n.On("Put", mock.Anything, key, mock.Anything).Run(func(args mock.Arguments) {
		saved = append([]byte(nil), args.Get(2).([]byte)...)
	}).Return(nil).Once()
	updated, err := s.SetHostWAF(ctx, &protocol.SetHostWAFRequest{
		Hostname: hostname, Policy: &protocol.WAFPolicy{Mode: protocol.WAFPolicy_OFF},
	})
	require.NoError(t, err)
	require.NotEmpty(t, updated.Version)
	n.On("Get", mock.Anything, key).Return(saved, nil).Times(3)
	policy, err := s.WAFPolicy(ctx, hostname)
	require.NoError(t, err)
	require.Equal(t, protocol.WAFPolicy_OFF, policy.Mode)
	_, err = s.SetHostWAF(ctx, &protocol.SetHostWAFRequest{
		Hostname: hostname, Policy: &protocol.WAFPolicy{Mode: protocol.WAFPolicy_MONITOR},
	})
	require.Equal(t, twirp.FailedPrecondition, err.(twirp.Error).Code())

	// A saved exception loses authority when the hostname leaves its owner,
	// even if the record remains and a route is still cached elsewhere.
	n.On("PrefixContains", mock.Anything, prefix, []byte(hostname)).Return(false, nil).Once()
	policy, err = s.WAFPolicy(ctx, hostname)
	require.NoError(t, err)
	require.Equal(t, protocol.WAFPolicy_INHERIT, policy.Mode)
	n.AssertExpectations(t)
}

func TestHostWAFPreviousOwnerAndCustomAuthority(t *testing.T) {
	s, n, _, cert := sessionFixture(t)
	ctx, _ := sessionContext(t, cert)
	owner, client, err := extractAuthenticated(ctx)
	require.NoError(t, err)
	hostname := "app.example.com"
	previousOwner := &protocol.ClientToken{Token: []byte("previous-owner")}
	data, err := waf.MarshalJSON(&protocol.HostWAFRecord{Schema: 1, Owner: previousOwner.Token, Policy: &protocol.WAFPolicy{Mode: protocol.WAFPolicy_OFF}})
	require.NoError(t, err)
	custom, err := (&protocol.CustomHostname{ClientIdentity: client, ClientToken: owner}).MarshalVT()
	require.NoError(t, err)
	n.On("PrefixContains", mock.Anything, []byte(tun.ClientHostnamesPrefix(owner)), []byte(hostname)).Return(true, nil).Once()
	n.On("Get", mock.Anything, []byte(tun.CustomHostnameKey(hostname))).Return(custom, nil).Twice()
	n.On("Get", mock.Anything, []byte(tun.HostWAFKey(hostname))).Return(data, nil).Twice()
	response, err := s.GetHostWAF(ctx, &protocol.GetHostWAFRequest{Hostname: hostname})
	require.NoError(t, err)
	require.Empty(t, response.Version)
	require.Equal(t, protocol.WAFPolicy_INHERIT, response.Policy.Mode)

	// Even a leftover registration prefix cannot override the current custom
	// hostname owner.
	n.On("PrefixContains", mock.Anything, []byte(tun.ClientHostnamesPrefix(previousOwner)), []byte(hostname)).Return(true, nil).Once()
	policy, err := s.WAFPolicy(ctx, hostname)
	require.NoError(t, err)
	require.Equal(t, protocol.WAFPolicy_INHERIT, policy.Mode)
	n.AssertExpectations(t)
}

func TestHostWAFRejectsOtherOwners(t *testing.T) {
	for _, hostname := range []string{"unowned-label", "app.example.com"} {
		t.Run(hostname, func(t *testing.T) {
			s, n, _, cert := sessionFixture(t)
			ctx, _ := sessionContext(t, cert)
			owner, _, err := extractAuthenticated(ctx)
			require.NoError(t, err)
			custom := strings.Contains(hostname, ".")
			n.On("PrefixContains", mock.Anything, []byte(tun.ClientHostnamesPrefix(owner)), []byte(hostname)).Return(custom, nil).Twice()
			if custom {
				data, err := (&protocol.CustomHostname{ClientToken: &protocol.ClientToken{Token: []byte("another-owner")}}).MarshalVT()
				require.NoError(t, err)
				n.On("Get", mock.Anything, []byte(tun.CustomHostnameKey(hostname))).Return(data, nil).Twice()
			}
			lease := []byte(tun.ClientLeaseKey(owner))
			n.On("Acquire", mock.Anything, lease, mock.Anything).Return(uint64(1), nil).Once()
			n.On("Release", mock.Anything, lease, uint64(1)).Return(nil).Once()
			_, err = s.GetHostWAF(ctx, &protocol.GetHostWAFRequest{Hostname: hostname})
			require.Equal(t, twirp.PermissionDenied, err.(twirp.Error).Code())
			_, err = s.SetHostWAF(ctx, &protocol.SetHostWAFRequest{Hostname: hostname, Policy: &protocol.WAFPolicy{Mode: protocol.WAFPolicy_OFF}})
			require.Equal(t, twirp.PermissionDenied, err.(twirp.Error).Code())
			n.AssertNotCalled(t, "Put", mock.Anything, mock.Anything, mock.Anything)
			n.AssertExpectations(t)
		})
	}
}

func TestHostWAFRPCRejectsUnknownModes(t *testing.T) {
	s, n, _, cert := sessionFixture(t)
	ctx, _ := sessionContext(t, cert)
	handler := protocol.NewTunnelServiceServer(s)
	for _, mode := range []string{`"FUTURE_MODE"`, `99`, `0`} {
		t.Run(mode, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, protocol.TunnelServicePathPrefix+"SetHostWAF", strings.NewReader(`{"hostname":"test","policy":{"mode":`+mode+`}}`))
			r.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r.WithContext(ctx))
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, response.Body.String(), `"code":"invalid_argument"`)
		})
	}
	n.AssertNotCalled(t, "Put", mock.Anything, mock.Anything, mock.Anything)
}

func TestReleaseTunnelClearsWAFBeforeOwnership(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "reset fails"
		}
		t.Run(name, func(t *testing.T) {
			s, n, _, cert := sessionFixture(t)
			ctx, _ := sessionContext(t, cert)
			owner, _, err := extractAuthenticated(ctx)
			require.NoError(t, err)
			hostname := "test"
			prefix := []byte(tun.ClientHostnamesPrefix(owner))
			lease := []byte(tun.ClientLeaseKey(owner))
			n.On("Acquire", mock.Anything, lease, mock.Anything).Return(uint64(1), nil).Once()
			n.On("Release", mock.Anything, lease, uint64(1)).Return(nil).Once()
			n.On("PrefixContains", mock.Anything, prefix, []byte(hostname)).Return(true, nil).Once()
			for i := range tun.NumRedundantLinks {
				n.On("Delete", mock.Anything, []byte(tun.RoutingKey(hostname, i+1))).Return(nil).Once()
			}
			data, err := waf.MarshalJSON(&protocol.HostWAFRecord{Schema: 1, Owner: owner.Token, Policy: &protocol.WAFPolicy{Mode: protocol.WAFPolicy_OFF}})
			require.NoError(t, err)
			n.On("Get", mock.Anything, []byte(tun.HostWAFKey(hostname))).Return(data, nil).Once()
			var resetErr error
			if fail {
				resetErr = errors.New("storage unavailable")
			}
			reset := n.On("Delete", mock.Anything, []byte(tun.HostWAFKey(hostname))).Return(resetErr).Once()
			if !fail {
				n.On("PrefixRemove", mock.Anything, prefix, []byte(hostname)).Return(nil).Once().NotBefore(reset)
				n.On("Delete", mock.Anything, []byte(tun.CustomHostnameKey(hostname))).Return(nil).Once()
			}
			_, err = s.ReleaseTunnel(ctx, &protocol.ReleaseTunnelRequest{Hostname: hostname})
			if fail {
				require.Error(t, err)
				n.AssertNotCalled(t, "PrefixRemove", mock.Anything, mock.Anything, mock.Anything)
			} else {
				require.NoError(t, err)
			}
			n.AssertExpectations(t)
		})
	}
}

func TestHostWAFRPCBodyLimit(t *testing.T) {
	handler := limitClientRPCBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	}))
	for _, tc := range []struct {
		method string
		size   int
		status int
	}{
		{"SetHostWAF", 2 << 10, http.StatusOK},
		{"SetHostWAF", maxHostWAFBytes + 1, http.StatusRequestEntityTooLarge},
		{"PublishTunnel", 2 << 10, http.StatusRequestEntityTooLarge},
	} {
		r := httptest.NewRequest(http.MethodPost, protocol.TunnelServicePathPrefix+tc.method, strings.NewReader(strings.Repeat("x", tc.size)))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		require.Equal(t, tc.status, response.Code)
	}
}
