package overlay

import (
	"context"
	"time"

	"specter/spec/protocol"

	"github.com/lucas-clemente/quic-go"
)

type Transport struct {
	self *protocol.Node
}

func (t *Transport) Identifty() *protocol.Node {
	return t.self
}

func (t *Transport) Dial(ctx context.Context, peer *protocol.Node) (quic.Stream, error) {
	dialCtx, dialCancel := context.WithTimeout(ctx, time.Second*3)
	defer dialCancel()

	q, err := quic.DialAddrContext(dialCtx, peer.GetAddress(), nil, nil)
	if err != nil {
		return nil, err
	}

	openCtx, openCancel := context.WithTimeout(ctx, time.Microsecond)
	defer openCancel()

	return q.OpenStreamSync(openCtx)
}

func (t *Transport) Accept(ctx context.Context, handleFn func(quic.Connection)) error {
	l, err := quic.ListenAddr(t.self.GetAddress(), nil, nil)
	if err != nil {
		return err
	}
	for {
		q, err := l.Accept(ctx)
		if err != nil {
			return err
		}
		go handleFn(q)
	}
}
