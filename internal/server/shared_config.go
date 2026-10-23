package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	chordImpl "go.miragespace.co/specter/chord"
	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/spec/transport"
	"go.miragespace.co/specter/util"

	"go.uber.org/zap"
)

// sharedConfigPeers returns graph edges, not a claimed inventory of the cluster.
// Include every local vnode's successors so cache eviction cannot remove the
// only synchronization path to another physical server.
func sharedConfigPeers(tp transport.Transport, nodes []*chordImpl.LocalNode, join *protocol.Node) []*protocol.Node {
	peers := make([]*protocol.Node, 0)
	for _, peer := range tp.ListConnected() {
		peers = append(peers, peer.Identity)
	}
	for _, node := range nodes {
		successors, err := node.GetSuccessors()
		if err != nil {
			continue
		}
		for _, successor := range successors {
			peers = append(peers, successor.Identity())
		}
	}
	if join != nil {
		peers = append(peers, join)
	}
	return peers
}

func localRPCHandler(rpc http.Handler, service protocol.ConfigService) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(protocol.ConfigServicePathPrefix, config.ReadHandler(service))
	mux.Handle("/", rpc)
	return mux
}

func startLocalRPC(ctx context.Context, listener net.Listener, rpc http.Handler, service protocol.ConfigService, logger *zap.Logger) func() {
	srv := &http.Server{
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
		Handler:           localRPCHandler(rpc, service),
		ErrorLog:          util.GetStdLogger(logger, "rpc_external"),
	}
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			logger.Error("Local RPC stopped", zap.Error(err))
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = srv.Close() })
	return func() {
		stop()
		_ = srv.Close()
	}
}
