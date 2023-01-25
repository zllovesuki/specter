package router

import (
	"context"
	"sync"

	"go.uber.org/zap"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"
)

type StreamHandler func(delegate *transport.StreamDelegate)

type StreamRouter struct {
	logger         *zap.Logger
	chordHandlers  sync.Map
	clientHandlers sync.Map
	chordStream    <-chan *transport.StreamDelegate
	clientStream   <-chan *transport.StreamDelegate
}

func NewStreamRouter(logger *zap.Logger, chordTransport, clientTransport transport.Transport) *StreamRouter {
	router := &StreamRouter{
		logger: logger,
	}
	if chordTransport != nil {
		router.chordStream = chordTransport.AcceptStream()
	}
	if clientTransport != nil {
		router.clientStream = clientTransport.AcceptStream()
	}
	return router
}

func (s *StreamRouter) AttachChord(kind protocol.Stream_Type, handler StreamHandler) {
	s.chordHandlers.Store(kind, handler)
}

func (s *StreamRouter) AttachClient(kind protocol.Stream_Type, handler StreamHandler) {
	s.clientHandlers.Store(kind, handler)
}

func (s *StreamRouter) acceptChord(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case delegate := <-s.chordStream:
			go func(delegate *transport.StreamDelegate) {
				handler, ok := s.chordHandlers.Load(delegate.Kind)
				if !ok {
					s.logger.Warn("No handler found for chord transport delegate",
						zap.String("peer", delegate.Identity.String()),
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
			go func(delegate *transport.StreamDelegate) {
				handler, ok := s.clientHandlers.Load(delegate.Kind)
				if !ok {
					s.logger.Warn("No handler found for client transport delegate",
						zap.String("peer", delegate.Identity.String()),
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
