package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.miragespace.co/specter/spec/mocks"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
)

func TestDelegationLifecycle(t *testing.T) {
	t.Run("owner authentication", func(t *testing.T) {
		s, n, _, cert := sessionFixture(t)
		tp := mocks.SelfTransport()
		tp.WithCertificate(cert)
		router := transport.NewStreamRouter(s.Logger, nil, tp)
		s.AttachRouter(t.Context(), router)
		go router.Accept(t.Context())
		cli := rpc.DynamicTunnelClient(rpc.DisablePooling(t.Context()), tp)
		n.On("Get", mock.Anything, mock.Anything).Return(nil, nil)
		ctx := rpc.WithNode(t.Context(), &protocol.Node{Address: "client"})
		_, err := cli.MintDelegation(ctx, &protocol.MintDelegationRequest{Hostname: "test"})
		require.Equal(t, twirp.Unauthenticated, err.(twirp.Error).Code())
		_, err = cli.ListDelegations(ctx, &protocol.ListDelegationsRequest{})
		require.Equal(t, twirp.Unauthenticated, err.(twirp.Error).Code())
		_, err = cli.RevokeDelegation(ctx, &protocol.RevokeDelegationRequest{})
		require.Equal(t, twirp.Unauthenticated, err.(twirp.Error).Code())
		_, err = cli.PublishTunnel(ctx, &protocol.PublishTunnelRequest{
			Hostname: "test",
			Servers:  []*protocol.Node{{Address: "server"}},
		})
		require.Equal(t, twirp.Unauthenticated, err.(twirp.Error).Code())
		n.AssertNotCalled(t, "Acquire", mock.Anything, mock.Anything, mock.Anything)
		n.AssertNotCalled(t, "Put", mock.Anything, mock.Anything, mock.Anything)
	})
	t.Run("mint list revoke", func(t *testing.T) {
		s, n, _, cert := sessionFixture(t)
		ctx, _ := sessionContext(t, cert)
		owner, _, err := extractAuthenticated(ctx)
		require.NoError(t, err)
		index := []byte(tun.ClientDelegationsPrefix(owner))
		n.On("PrefixContains", mock.Anything, []byte(tun.ClientHostnamesPrefix(owner)), []byte("test")).Return(true, nil)
		n.On("PrefixList", mock.Anything, index).Return([][]byte{}, nil).Once()
		var rec protocol.DelegationRecord
		appended := n.On("PrefixAppend", mock.Anything, index, mock.Anything).Return(nil).Once()
		n.On("Put", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			require.NoError(t, rec.UnmarshalVT(args.Get(2).([]byte)))
			require.Equal(t, []byte(tun.DelegationKey(rec.Id)), args.Get(1))
		}).Return(nil).Once().NotBefore(appended)
		minted, err := s.MintDelegation(ctx, &protocol.MintDelegationRequest{Hostname: "test"})
		require.NoError(t, err)
		secret, err := tun.ParseDelegationToken(minted.Token)
		require.NoError(t, err)
		require.Equal(t, tun.DelegationID(secret), minted.Grant.Id)
		require.Zero(t, minted.Grant.ExpiresAt)
		missing := strings.Repeat("0", 64)
		n.On("PrefixList", mock.Anything, index).Return([][]byte{[]byte(rec.Id), []byte(missing)}, nil).Once()
		data, _ := rec.MarshalVT()
		n.On("Get", mock.Anything, []byte(tun.DelegationKey(rec.Id))).Return(data, nil).Twice()
		n.On("Get", mock.Anything, []byte(tun.DelegationKey(missing))).Return(nil, nil).Once()
		listed, err := s.ListDelegations(ctx, &protocol.ListDelegationsRequest{})
		require.NoError(t, err)
		require.Len(t, listed.Grants, 2)
		require.True(t, listed.Grants[1].Incomplete)
		require.NotContains(t, listed.String(), "tg1_")
		deleted := n.On("Delete", mock.Anything, []byte(tun.DelegationKey(rec.Id))).Return(nil).Once()
		n.On("PrefixRemove", mock.Anything, index, []byte(rec.Id)).Return(errors.New("index unavailable")).Once().NotBefore(deleted)
		revoked, err := s.RevokeDelegation(ctx, &protocol.RevokeDelegationRequest{Id: rec.Id})
		require.NoError(t, err)
		require.True(t, revoked.Revoked)
		require.NotEmpty(t, revoked.IndexError)
		n.On("Delete", mock.Anything, []byte(tun.DelegationKey(rec.Id))).Return(nil).Once()
		n.On("Get", mock.Anything, []byte(tun.DelegationKey(rec.Id))).Return(nil, nil).Once()
		n.On("PrefixContains", mock.Anything, index, []byte(rec.Id)).Return(true, nil).Once()
		n.On("PrefixRemove", mock.Anything, index, []byte(rec.Id)).Return(nil).Once()
		revoked, err = s.RevokeDelegation(ctx, &protocol.RevokeDelegationRequest{Id: rec.Id})
		require.NoError(t, err)
		require.True(t, revoked.Revoked)
		require.Empty(t, revoked.IndexError)
		_, err = s.MintDelegation(ctx, &protocol.MintDelegationRequest{
			Hostname:  "test",
			ExpiresAt: time.Now().Add(-time.Hour).Unix(),
		})
		require.Equal(t, twirp.InvalidArgument, err.(twirp.Error).Code())
		n.On("PrefixList", mock.Anything, index).Return(make([][]byte, 128), nil).Once()
		_, err = s.MintDelegation(ctx, &protocol.MintDelegationRequest{Hostname: "test"})
		require.Equal(t, twirp.ResourceExhausted, err.(twirp.Error).Code())
		n.AssertExpectations(t)
	})
}
