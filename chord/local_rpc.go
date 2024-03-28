package chord

import (
	"context"
	"errors"
	"net"
	"net/http"

	"go.miragespace.co/specter/spec/chord"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/rpc"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/util"
	"go.miragespace.co/specter/util/ratecounter"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

func (n *LocalNode) logError(ctx context.Context, err twirp.Error) context.Context {
	if err.Code() == twirp.FailedPrecondition || errors.Is(err, chord.ErrNodeGone) {
		return ctx
	}
	delegation := rpc.GetDelegation(ctx)
	if delegation != nil {
		service, _ := twirp.ServiceName(ctx)
		method, _ := twirp.MethodName(ctx)
		l := n.logger.With(
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
		n.rpcErrorCount.Inc()
	}
	return ctx
}

func (n *LocalNode) getIncrementor(rate *ratecounter.Rate) func(ctx context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		rate.Increment()
		return ctx, nil
	}
}

func (n *LocalNode) getRPCHandler(ctx context.Context) http.Handler {
	n.rpcHandlerOnce.Do(func() {
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
		n.rpcHandler = rpcHandler
	})

	return n.rpcHandler
}

func (n *LocalNode) AttachRouter(ctx context.Context, router *transport.StreamRouter) {
	n.stopWg.Add(1)

	rpcHandler := n.getRPCHandler(ctx)
	srv := &http.Server{
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return rpc.WithDelegation(ctx, c.(*transport.StreamDelegate))
		},
		ReadHeaderTimeout: rpcTimeout,
		Handler:           rpcHandler,
		ErrorLog:          util.GetStdLogger(n.logger, "rpc_server"),
	}

	go srv.Serve(n.rpcAcceptor)
	go func() {
		defer n.stopWg.Done()

		<-n.stopCh
		n.rpcAcceptor.Close()
	}()

	router.HandleChord(protocol.Stream_RPC, n.Identity(), func(delegate *transport.StreamDelegate) {
		n.rpcAcceptor.Handle(delegate)
	})
}

func (n *LocalNode) AttachRoot(ctx context.Context, router *transport.StreamRouter) {
	router.HandleChord(protocol.Stream_RPC, nil, func(delegate *transport.StreamDelegate) {
		n.rpcAcceptor.Handle(delegate)
	})
}

func (n *LocalNode) AttachExternal(ctx context.Context, listener net.Listener) {
	rpcHandler := n.getRPCHandler(ctx)
	srv := &http.Server{
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		ReadHeaderTimeout: rpcTimeout,
		Handler:           rpcHandler,
		ErrorLog:          util.GetStdLogger(n.logger, "rpc_external"),
	}
	go srv.Serve(listener)
}
