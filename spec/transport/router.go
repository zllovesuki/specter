package transport

import (
	"context"
	"sync"

	"kon.nect.sh/specter/spec/protocol"

	"github.com/zhangyunhao116/skipmap"
	"go.uber.org/zap"
)

type StreamHandler func(delegate *StreamDelegate)

type StreamRouter struct {
	logger                *zap.Logger
	virtualChordHandlers  *skipmap.Int32Map[*sync.Map] // map[protocol.Stream_Type]StreamHandler
	physicalChordHandlers sync.Map                     // map[protocol.Stream_Type]StreamHandler
	tunnelHandlers        sync.Map                     // map[protocol.Stream_Type]StreamHandler
	chordStream           <-chan *StreamDelegate
	tunnelStream          <-chan *StreamDelegate
}

func NewStreamRouter(logger *zap.Logger, chordTransport, tunnelTransport Transport) *StreamRouter {
	router := &StreamRouter{
		logger:               logger,
		virtualChordHandlers: skipmap.NewInt32[*sync.Map](),
	}
	if chordTransport != nil {
		router.chordStream = chordTransport.AcceptStream()
	}
	if tunnelTransport != nil {
		router.tunnelStream = tunnelTransport.AcceptStream()
	}
	return router
}

func (s *StreamRouter) HandleChord(kind protocol.Stream_Type, target *protocol.Node, handler StreamHandler) {
	if target == nil {
		// physical node handler
		s.physicalChordHandlers.Store(kind, handler)
	} else {
		// virtual node handler
		m, _ := s.virtualChordHandlers.LoadOrStoreLazy(int32(kind), func() *sync.Map {
			return &sync.Map{}
		})
		m.Store(target.GetId(), handler)
	}
}

func (s *StreamRouter) HandleTunnel(kind protocol.Stream_Type, handler StreamHandler) {
	s.tunnelHandlers.Store(kind, handler)
}

func (s *StreamRouter) acceptChord(ctx context.Context) {
	var (
		m       *sync.Map
		handler any
		ok      bool
	)
	for {
		select {
		case <-ctx.Done():
			return
		case delegate := <-s.chordStream:
			m, ok = s.virtualChordHandlers.Load(int32(delegate.Kind))
			if ok {
				// prioritize specific virtual node handler
				handler, ok = m.Load(delegate.Identity.GetId())
				if !ok {
					// fallback to root handler
					handler, ok = s.physicalChordHandlers.Load(delegate.Kind)
				}
			} else {
				// otherwise physical node handler
				handler, ok = s.physicalChordHandlers.Load(delegate.Kind)
			}
			if !ok {
				s.logger.Warn("No handler found for chord transport delegate",
					zap.Object("peer", delegate.Identity),
					zap.String("kind", delegate.Kind.String()),
				)
				delegate.Close()
				continue
			}
			go handler.(StreamHandler)(delegate)
		}
	}
}

func (s *StreamRouter) acceptTunnel(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case delegate := <-s.tunnelStream:
			handler, ok := s.tunnelHandlers.Load(delegate.Kind)
			if !ok {
				s.logger.Warn("No handler found for tunnel transport delegate",
					zap.Object("peer", delegate.Identity),
					zap.String("kind", delegate.Kind.String()),
				)
				delegate.Close()
				continue
			}
			go handler.(StreamHandler)(delegate)
		}
	}
}

func (s *StreamRouter) Accept(ctx context.Context) {
	if s.chordStream != nil {
		go s.acceptChord(ctx)
		go s.acceptChord(ctx)
	}
	if s.tunnelStream != nil {
		go s.acceptTunnel(ctx)
		go s.acceptTunnel(ctx)
	}
}
