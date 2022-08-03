package server

import (
	"fmt"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"

	"go.uber.org/zap"
)

// TODO: cleanup the retry semantics
const (
	kvRetryInterval = time.Second * 2
)

func (s *Server) publishIdentities() error {
	identities := &protocol.IdentitiesPair{
		Chord: s.chordTransport.Identity(),
		Tun:   s.clientTransport.Identity(),
	}
	buf, err := identities.MarshalVT()
	if err != nil {
		return err
	}

	keys := []string{
		tun.IdentitiesChordKey(s.chordTransport.Identity()),
		tun.IdentitiesTunKey(s.clientTransport.Identity()),
	}
	for _, key := range keys {
	RETRY:
		err := s.chord.Put([]byte(key), buf)
		if err != nil {
			if chord.ErrorIsRetryable(err) {
				time.Sleep(kvRetryInterval)
				goto RETRY
			}
			return err
		}
	}

	s.logger.Info("identities published on chord",
		zap.String("chord", keys[0]),
		zap.String("tun", keys[1]))

	return nil
}

func (s *Server) unpublishIdentities() {
	keys := []string{
		tun.IdentitiesChordKey(s.chordTransport.Identity()),
		tun.IdentitiesTunKey(s.clientTransport.Identity()),
	}
	for _, key := range keys {
	RETRY:
		err := s.chord.Delete([]byte(key))
		if err != nil {
			if chord.ErrorIsRetryable(err) {
				time.Sleep(kvRetryInterval)
				goto RETRY
			}
			return
		}
	}

	s.logger.Info("identities unpublished on chord",
		zap.String("chord", keys[0]),
		zap.String("tun", keys[1]))
}

func (s *Server) lookupIdentities(key string) (*protocol.IdentitiesPair, error) {
RETRY:
	buf, err := s.chord.Get([]byte(key))
	if err != nil {
		if chord.ErrorIsRetryable(err) {
			time.Sleep(kvRetryInterval)
			goto RETRY
		}
		return nil, err
	}
	if len(buf) == 0 {
		return nil, fmt.Errorf("no identities pair found with key: %s", key)
	}
	identities := &protocol.IdentitiesPair{}
	if err := identities.UnmarshalVT(buf); err != nil {
		return nil, fmt.Errorf("identities decode failure: %w", err)
	}
	return identities, nil
}
