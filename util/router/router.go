package router

import (
	"context"

	"go.uber.org/zap"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/transport"

	"github.com/zhangyunhao116/skipmap"
)

type StreamHandler func(delegate *transport.StreamDelegate)

type StreamRouter struct {
	logger         *zap.Logger
	chordHandlers  *skipmap.FuncMap[protocol.Stream_Type, StreamHandler]
	clientHandlers *skipmap.FuncMap[protocol.Stream_Type, StreamHandler]
	chordStream    <-chan *transport.StreamDelegate
	clientStream   <-chan *transport.StreamDelegate
}

func NewStreamRouter(logger *zap.Logger, chordTransport, clientTransport transport.Transport) *StreamRouter {
	router := &StreamRouter{
		logger: logger,
		chordHandlers: skipmap.NewFunc[protocol.Stream_Type, StreamHandler](func(a, b protocol.Stream_Type) bool {
			return a < b
		}),
		clientHandlers: skipmap.NewFunc[protocol.Stream_Type, StreamHandler](func(a, b protocol.Stream_Type) bool {
			return a < b
		}),
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

func (s *StreamRouter) Accept(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case delegate := <-s.chordStream:
			go s.handleDelegation(s.chordHandlers, delegate)
		case delegate := <-s.clientStream:
			go s.handleDelegation(s.clientHandlers, delegate)
		}
	}
}

func (s *StreamRouter) handleDelegation(handlers *skipmap.FuncMap[protocol.Stream_Type, StreamHandler], delegate *transport.StreamDelegate) {
	handler, ok := handlers.Load(delegate.Kind)
	if !ok {
		s.logger.Warn("No handler found", zap.String("kind", delegate.Kind.String()))
		return
	}
	handler(delegate)
}
