package transport

import (
	"context"
	"sync"

	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/zap"
)

type StreamHandler func(delegate *StreamDelegate)

type StreamRouter struct {
	logger         *zap.Logger
	chordHandlers  sync.Map
	tunnelHandlers sync.Map
	chordStream    <-chan *StreamDelegate
	clientStream   <-chan *StreamDelegate
}

func NewStreamRouter(logger *zap.Logger, chordTransport, tunnelTransport Transport) *StreamRouter {
	router := &StreamRouter{
		logger: logger,
	}
	if chordTransport != nil {
		router.chordStream = chordTransport.AcceptStream()
	}
	if tunnelTransport != nil {
		router.clientStream = tunnelTransport.AcceptStream()
	}
	return router
}

func (s *StreamRouter) HandleChord(kind protocol.Stream_Type, handler StreamHandler) {
	s.chordHandlers.Store(kind, handler)
}

func (s *StreamRouter) HandleTunnel(kind protocol.Stream_Type, handler StreamHandler) {
	s.tunnelHandlers.Store(kind, handler)
}

func (s *StreamRouter) acceptChord(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case delegate := <-s.chordStream:
			go func(delegate *StreamDelegate) {
				handler, ok := s.chordHandlers.Load(delegate.Kind)
				if !ok {
					s.logger.Warn("No handler found for chord transport delegate",
						zap.Object("peer", delegate.Identity),
						zap.String("kind", delegate.Kind.String()),
					)
					return
				}
				handler.(StreamHandler)(delegate)
			}(delegate)
		}
	}
}

func (s *StreamRouter) acceptClient(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case delegate := <-s.clientStream:
			go func(delegate *StreamDelegate) {
				handler, ok := s.tunnelHandlers.Load(delegate.Kind)
				if !ok {
					s.logger.Warn("No handler found for tunnel transport delegate",
						zap.Object("peer", delegate.Identity),
						zap.String("kind", delegate.Kind.String()),
					)
					return
				}
				handler.(StreamHandler)(delegate)
			}(delegate)
		}
	}
}

func (s *StreamRouter) Accept(ctx context.Context) {
	if s.chordStream != nil {
		go s.acceptChord(ctx)
	}
	if s.clientStream != nil {
		go s.acceptClient(ctx)
	}
}
