package acme

import (
	"bytes"
	"context"
	"testing"

	acmeSpec "kon.nect.sh/specter/spec/acme"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/mocks"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"

	"github.com/mholt/acmez/acme"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testManagedDomain = "acme.example.com"
	testDynamicDomain = "dynamic.example.com"
)

func TestSolverManaged(t *testing.T) {
	as := require.New(t)
	mockKV := new(mocks.VNode)

	solver := &ChordSolver{
		KV:             mockKV,
		ManagedDomains: []string{testManagedDomain},
	}

	append := mockKV.On("PrefixAppend", mock.Anything, mock.MatchedBy(func(key []byte) bool {
		return bytes.Equal([]byte(dnsKeyName(acmeSpec.ManagedDelegation)), key)
	}), mock.Anything).Return(nil)

	mockKV.On("PrefixRemove", mock.Anything, mock.MatchedBy(func(key []byte) bool {
		return bytes.Equal([]byte(dnsKeyName(acmeSpec.ManagedDelegation)), key)
	}), mock.Anything).Return(nil).NotBefore(append)

	err := solver.Present(context.Background(), acme.Challenge{
		Identifier: acme.Identifier{
			Type:  "dns",
			Value: testManagedDomain,
		},
	})
	as.NoError(err)

	err = solver.CleanUp(context.Background(), acme.Challenge{
		Identifier: acme.Identifier{
			Type:  "dns",
			Value: testManagedDomain,
		},
	})
	as.NoError(err)

	mockKV.AssertExpectations(t)
}

func TestSolverDynamic(t *testing.T) {
	as := require.New(t)
	mockKV := new(mocks.VNode)

	solver := &ChordSolver{
		KV:             mockKV,
		ManagedDomains: []string{testManagedDomain},
	}

	testClient := &protocol.Node{
		Id:      chord.Random(),
		Address: "addr",
	}
	testToken := &protocol.ClientToken{
		Token: []byte("token"),
	}
	testBundle := &protocol.CustomHostname{
		ClientIdentity: testClient,
		ClientToken:    testToken,
	}
	bundleBuf, err := testBundle.MarshalVT()
	as.NoError(err)

	get := mockKV.On("Get", mock.Anything, mock.MatchedBy(func(key []byte) bool {
		return bytes.Equal([]byte(tun.CustomHostnameKey(testDynamicDomain)), key)
	})).Return(bundleBuf, nil)

	append := mockKV.On("PrefixAppend", mock.Anything, mock.MatchedBy(func(key []byte) bool {
		return bytes.Equal([]byte(dnsKeyName(acmeSpec.EncodeClientToken(testToken.GetToken()))), key)
	}), mock.Anything).Return(nil).NotBefore(get)

	mockKV.On("PrefixRemove", mock.Anything, mock.MatchedBy(func(key []byte) bool {
		return bytes.Equal([]byte(dnsKeyName(acmeSpec.EncodeClientToken(testToken.GetToken()))), key)
	}), mock.Anything).Return(nil).NotBefore(append)

	err = solver.Present(context.Background(), acme.Challenge{
		Identifier: acme.Identifier{
			Type:  "dns",
			Value: testDynamicDomain,
		},
	})
	as.NoError(err)

	err = solver.CleanUp(context.Background(), acme.Challenge{
		Identifier: acme.Identifier{
			Type:  "dns",
			Value: testDynamicDomain,
		},
	})
	as.NoError(err)

	mockKV.AssertExpectations(t)
}
