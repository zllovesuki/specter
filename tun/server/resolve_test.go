package server

import (
	"context"
	"testing"
	"time"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIdentitiesRoutine(t *testing.T) {
	as := require.New(t)

	_, node, clientT, chordT, serv := getFixture(t, as)
	_, cht, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chordT.On("Identity").Return(cht)
	clientT.On("Identity").Return(tn)

	// On start up -> publish
	node.On("Put", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		exp := [][]byte{
			[]byte(tun.IdentitiesChordKey(cht)),
			[]byte(tun.IdentitiesTunnelKey(tn)),
		}
		return assertBytes(k, exp...)
	}), mock.MatchedBy(func(v []byte) bool {
		pair := &protocol.IdentitiesPair{}
		err := pair.UnmarshalVT(v)
		if err != nil {
			return false
		}
		if pair.GetChord().GetId() != cht.GetId() {
			return false
		}
		if pair.GetTun().GetId() != tn.GetId() {
			return false
		}
		return true
	})).Return(nil)

	// On stop -> unpublish
	node.On("Delete", mock.Anything, mock.MatchedBy(func(k []byte) bool {
		exp := [][]byte{
			[]byte(tun.IdentitiesChordKey(cht)),
			[]byte(tun.IdentitiesTunnelKey(tn)),
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
