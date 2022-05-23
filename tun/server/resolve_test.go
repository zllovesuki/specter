package server

import (
	"context"
	"testing"
	"time"

	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/transport"
	"github.com/zllovesuki/specter/spec/tun"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIdentitiesRoutine(t *testing.T) {
	as := require.New(t)

	_, node, clientT, chordT, serv := getFixture(as)
	_, cht, tn := getIdentities()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chordChan := make(chan *transport.StreamDelegate)
	chordT.On("Direct").Return(chordChan)
	chordT.On("Identity").Return(cht)

	clientChan := make(chan *transport.StreamDelegate)
	clientT.On("Direct").Return(clientChan)
	clientT.On("Identity").Return(tn)

	// On start up -> publish
	node.On("Put", mock.MatchedBy(func(k []byte) bool {
		exp := [][]byte{
			[]byte(tun.IdentitiesChordKey(cht)),
			[]byte(tun.IdentitiesTunKey(tn)),
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
	node.On("Delete", mock.MatchedBy(func(k []byte) bool {
		exp := [][]byte{
			[]byte(tun.IdentitiesChordKey(cht)),
			[]byte(tun.IdentitiesTunKey(tn)),
		}
		return assertBytes(k, exp...)
	})).Return(nil)

	go serv.Accept(ctx)

	<-time.After(time.Millisecond * 100)

	serv.Stop()

	<-time.After(time.Millisecond * 100)

	node.AssertExpectations(t)
	clientT.AssertExpectations(t)
	chordT.AssertExpectations(t)
}
