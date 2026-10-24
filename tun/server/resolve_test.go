package server

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPublishDestinationsRetriesFailedKey(t *testing.T) {
	as := require.New(t)
	_, node, clientT, chordT, serv := getFixture(t, as)
	defer serv.routeCache.Close()
	defer serv.keylessCache.Close()
	_, cht, tn := getIdentities()
	chordT.On("Identity").Return(cht)
	clientT.On("Identity").Return(tn)

	payload, err := (&protocol.TunnelDestination{Chord: cht, Tunnel: tn}).MarshalVT()
	as.NoError(err)
	chordKey := []byte(tun.DestinationByChordKey(cht))
	tunnelKey := []byte(tun.DestinationByTunnelKey(tn))
	node.On("Put", mock.Anything, chordKey, payload).Return(nil).Once()
	node.On("Put", mock.Anything, tunnelKey, payload).
		Return(errors.New("transient publication failure")).Once()
	node.On("Put", mock.Anything, tunnelKey, payload).Return(nil).Once()

	as.NoError(serv.publishDestinations(t.Context()))
	node.AssertExpectations(t)
}

func TestPublishDestinationsExhaustsRetries(t *testing.T) {
	as := require.New(t)
	_, node, clientT, chordT, serv := getFixture(t, as)
	defer serv.routeCache.Close()
	defer serv.keylessCache.Close()
	_, cht, tn := getIdentities()
	chordT.On("Identity").Return(cht)
	clientT.On("Identity").Return(tn)

	failure := errors.New("permanent publication failure")
	node.On("Put", mock.Anything, []byte(tun.DestinationByChordKey(cht)), mock.Anything).
		Return(failure).Times(3)

	as.Same(failure, serv.publishDestinations(t.Context()))
	node.AssertExpectations(t)
}

func TestMustRegisterCancellationStopsBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		as := require.New(t)
		_, node, clientT, chordT, serv := getFixture(t, as)
		defer serv.routeCache.Close()
		defer serv.keylessCache.Close()
		_, cht, tn := getIdentities()
		chordT.On("Identity").Return(cht)
		clientT.On("Identity").Return(tn)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		attempted := make(chan struct{})
		var publishCtx context.Context
		node.On("Put", mock.Anything, []byte(tun.DestinationByChordKey(cht)), mock.Anything).
			Run(func(args mock.Arguments) {
				publishCtx = args.Get(0).(context.Context)
				close(attempted)
			}).Return(errors.New("transient publication failure")).Once()

		finished := make(chan any, 1)
		go func() {
			defer func() { finished <- recover() }()
			serv.MustRegister(ctx)
		}()
		<-attempted
		synctest.Wait()
		as.NoError(publishCtx.Err())
		cancel()

		as.Equal("Error publishing destinations", <-finished)
		as.ErrorIs(publishCtx.Err(), context.Canceled)
		node.AssertExpectations(t)
	})
}

func TestIdentitiesRoutine(t *testing.T) {
	as := require.New(t)

	_, node, clientT, chordT, serv := getFixture(t, as)
	_, cht, tn := getIdentities()

	ctx := t.Context()

	chordT.On("Identity").Return(cht)
	clientT.On("Identity").Return(tn)

	// On start up -> publish
	node.On("Put", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		exp := [][]byte{
			[]byte(tun.DestinationByChordKey(cht)),
			[]byte(tun.DestinationByTunnelKey(tn)),
		}
		return assertBytes(k, exp...)
	}), mock.MatchedBy(func(v []byte) bool {
		pair := &protocol.TunnelDestination{}
		err := pair.UnmarshalVT(v)
		if err != nil {
			return false
		}
		if pair.GetChord().GetId() != cht.GetId() {
			return false
		}
		if pair.GetTunnel().GetId() != tn.GetId() {
			return false
		}
		return true
	})).Return(nil)

	// On stop -> unpublish
	node.On("Delete", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		exp := [][]byte{
			[]byte(tun.DestinationByChordKey(cht)),
			[]byte(tun.DestinationByTunnelKey(tn)),
		}
		return assertBytes(k, exp...)
	})).Return(nil)

	serv.MustRegister(ctx)

	<-time.After(time.Millisecond * 100)

	serv.Stop()

	<-time.After(time.Millisecond * 100)

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}
