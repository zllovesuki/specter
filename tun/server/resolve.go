package server

import (
	"context"
	"fmt"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/tun"

	"github.com/avast/retry-go/v4"
	"go.uber.org/zap"
)

const (
	kvRetryInterval = time.Second * 2
)

func (s *Server) publishIdentities(ctx context.Context) error {
	identities := &protocol.IdentitiesPair{
		Chord: s.chordTransport.Identity(),
		Tun:   s.tunnelTransport.Identity(),
	}

	buf, err := identities.MarshalVT()
	if err != nil {
		return err
	}

	keys := []string{
		tun.IdentitiesChordKey(s.chordTransport.Identity()),
		tun.IdentitiesTunnelKey(s.tunnelTransport.Identity()),
	}

	if err := retry.Do(func() error {
		for _, key := range keys {
			err := s.chord.Put(ctx, []byte(key), buf)
			if err != nil {
				return err
			}
		}
		return nil
	},
		retry.Context(ctx),
		retry.LastErrorOnly(true),
		retry.Delay(kvRetryInterval),
		retry.RetryIf(chord.ErrorIsRetryable),
	); err != nil {
		return err
	}

	s.logger.Info("identities published on chord",
		zap.String("chord", keys[0]),
		zap.String("tun", keys[1]))

	return nil
}

func (s *Server) unpublishIdentities(ctx context.Context) {
	keys := []string{
		tun.IdentitiesChordKey(s.chordTransport.Identity()),
		tun.IdentitiesTunnelKey(s.tunnelTransport.Identity()),
	}

	if err := retry.Do(func() error {
		for _, key := range keys {
			err := s.chord.Delete(ctx, []byte(key))
			if err != nil {
				return err
			}
		}
		return nil
	},
		retry.Context(ctx),
		retry.LastErrorOnly(true),
		retry.Delay(kvRetryInterval),
		retry.RetryIf(chord.ErrorIsRetryable),
	); err != nil {
		s.logger.Error("failed to unpublish identifies on chord", zap.Error(err))
	}

	s.logger.Info("identities unpublished on chord",
		zap.String("chord", keys[0]),
		zap.String("tun", keys[1]))
}

func (s *Server) lookupIdentities(ctx context.Context, key string) (*protocol.IdentitiesPair, error) {
	buf, err := s.chord.Get(ctx, []byte(key))
	if err != nil {
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
