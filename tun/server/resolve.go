package server

import (
	"context"
	"fmt"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/tun"

	"go.uber.org/zap"
)

func (s *Server) publishDestinations(ctx context.Context) error {
	destinations := &protocol.TunnelDestination{
		Chord:  s.ChordTransport.Identity(),
		Tunnel: s.TunnelTransport.Identity(),
	}

	buf, err := destinations.MarshalVT()
	if err != nil {
		return err
	}

	keys := []string{
		tun.DestinationByChordKey(s.ChordTransport.Identity()),
		tun.DestinationByTunnelKey(s.TunnelTransport.Identity()),
	}

	for _, key := range keys {
		err := s.Chord.Put(ctx, []byte(key), buf)
		if err != nil {
			return err
		}
	}

	s.Logger.Info("Destinations published on chord",
		zap.String("chord", keys[0]),
		zap.String("tunnel", keys[1]),
	)

	return nil
}

func (s *Server) unpublishDestinations(ctx context.Context) {
	keys := []string{
		tun.DestinationByChordKey(s.ChordTransport.Identity()),
		tun.DestinationByTunnelKey(s.TunnelTransport.Identity()),
	}

	for _, key := range keys {
		err := s.Chord.Delete(ctx, []byte(key))
		if err != nil {
			s.Logger.Error("Failed to unpublish destinations on chord", zap.Error(err))
			return
		}
	}

	s.Logger.Info("Destination unpublished on chord",
		zap.String("chord", keys[0]),
		zap.String("tunnel", keys[1]))
}

func (s *Server) lookupDestination(ctx context.Context, key string) (*protocol.TunnelDestination, error) {
	buf, err := s.Chord.Get(ctx, []byte(key))
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
