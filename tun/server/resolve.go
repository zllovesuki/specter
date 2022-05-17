package server

import (
	"fmt"

	"github.com/zllovesuki/specter/spec/protocol"
	"github.com/zllovesuki/specter/spec/tun"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func (s *Server) publishIdentities() error {
	identities := &protocol.IdentitiesPair{
		Chord: s.chordTransport.Identity(),
		Tun:   s.clientTransport.Identity(),
	}
	buf, err := proto.Marshal(identities)
	if err != nil {
		return err
	}

	keys := []string{
		tun.IdentitiesChordKey(s.chordTransport.Identity()),
		tun.IdentitiesTunKey(s.clientTransport.Identity()),
	}
	for _, key := range keys {
		err := s.chord.Put([]byte(key), buf)
		if err != nil {
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
		s.chord.Delete([]byte(key))
	}

	s.logger.Info("identities unpublished on chord",
		zap.String("chord", keys[0]),
		zap.String("tun", keys[1]))
}

func (s *Server) lookupIdentities(key string) (*protocol.IdentitiesPair, error) {
	identities := &protocol.IdentitiesPair{}
	buf, err := s.chord.Get([]byte(key))
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, fmt.Errorf("no identities pair found with key: %s", key)
	}
	if err := proto.Unmarshal(buf, identities); err != nil {
		return nil, fmt.Errorf("identities decode failure: %w", err)
	}
	return identities, nil
}
