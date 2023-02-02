package chord

import (
	"context"
	"net"
	"net/http"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util"
	"kon.nect.sh/specter/util/ratecounter"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

func (n *LocalNode) logError(ctx context.Context, err twirp.Error) context.Context {
	if err.Code() == twirp.FailedPrecondition {
		return ctx
	}
	delegation := rpc.GetDelegation(ctx)
	if delegation != nil {
		service, _ := twirp.ServiceName(ctx)
		method, _ := twirp.MethodName(ctx)
		l := n.BaseLogger.With(
			zap.String("component", "rpc_server"),
			zap.Object("peer", delegation.Identity),
			zap.String("service", service),
			zap.String("method", method),
		)
		cause, key := rpc.GetErrorMeta(err)
		if cause != "" {
			l = l.With(zap.String("cause", cause))
		}
		if key != "" {
			l = l.With(zap.String("kv-key", key))
		}
		l.Error("Error handling RPC request", zap.Error(err))
	}
	return ctx
}

func (n *LocalNode) getIncrementor(rate *ratecounter.Rate) func(ctx context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		rate.Increment()
		return ctx, nil
	}
}

func (n *LocalNode) AttachRouter(ctx context.Context, router *transport.StreamRouter) {
	n.stopWg.Add(1)

	r := &Server{
		LocalNode: n,
		Factory: func(node *protocol.Node) (chord.VNode, error) {
			return NewRemoteNode(ctx, n.BaseLogger, n.ChordClient, node)
		},
	}
	nsTwirp := protocol.NewVNodeServiceServer(
		r,
		twirp.WithServerHooks(&twirp.ServerHooks{
			RequestReceived: n.getIncrementor(n.chordRate),
			Error:           n.logError,
		}),
	)
	ksTwirp := protocol.NewKVServiceServer(
		r,
		twirp.WithServerHooks(&twirp.ServerHooks{
			RequestReceived: n.getIncrementor(n.kvRate),
			Error:           n.logError,
		}),
	)

	rpcHandler := chi.NewRouter()
	rpcHandler.Use(middleware.Recoverer)
	rpcHandler.Mount(nsTwirp.PathPrefix(), rpc.ExtractContext(nsTwirp))
	rpcHandler.Mount(ksTwirp.PathPrefix(), rpc.ExtractContext(ksTwirp))

	srv := &http.Server{
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return rpc.WithDelegation(ctx, c.(*transport.StreamDelegate))
		},
		ReadTimeout: rpcTimeout,
		Handler:     rpcHandler,
		ErrorLog:    util.GetStdLogger(n.logger, "rpc_server"),
	}

	go srv.Serve(n.rpcAcceptor)
	go func() {
		defer n.stopWg.Done()

		<-n.stopCh
		n.rpcAcceptor.Close()
	}()

	router.HandleChord(protocol.Stream_RPC, func(delegate *transport.StreamDelegate) {
		n.rpcAcceptor.Handle(delegate)
	})
}
