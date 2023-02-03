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

func (s *Server) publishDestinations(ctx context.Context) error {
	destinations := &protocol.TunnelDestination{
		Chord:  s.chordTransport.Identity(),
		Tunnel: s.tunnelTransport.Identity(),
	}

	buf, err := destinations.MarshalVT()
	if err != nil {
		return err
	}

	keys := []string{
		tun.DestinationByChordKey(s.chordTransport.Identity()),
		tun.DestinationByTunnelKey(s.tunnelTransport.Identity()),
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

	s.logger.Info("Destinations published on chord",
		zap.String("chord", keys[0]),
		zap.String("tunnel", keys[1]))

	return nil
}

func (s *Server) unpublishDestinations(ctx context.Context) {
	keys := []string{
		tun.DestinationByChordKey(s.chordTransport.Identity()),
		tun.DestinationByTunnelKey(s.tunnelTransport.Identity()),
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
		s.logger.Error("Failed to unpublish destinations on chord", zap.Error(err))
	}

	s.logger.Info("Destination unpublished on chord",
		zap.String("chord", keys[0]),
		zap.String("tunnel", keys[1]))
}

func (s *Server) lookupDestination(ctx context.Context, key string) (*protocol.TunnelDestination, error) {
	buf, err := s.chord.Get(ctx, []byte(key))
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, fmt.Errorf("no destination found with key: %s", key)
	}
	dst := &protocol.TunnelDestination{}
	if err := dst.UnmarshalVT(buf); err != nil {
		return nil, fmt.Errorf("tunnel destination decode failure: %w", err)
	}
	return dst, nil
}
