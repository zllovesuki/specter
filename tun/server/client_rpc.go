package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/spec/tun"
	"kon.nect.sh/specter/util"
	"kon.nect.sh/specter/util/promise"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sethvargo/go-diceware/diceware"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
	"kon.nect.sh/httprate"
)

var generator, _ = diceware.NewGenerator(nil)

const (
	testDatagramData = "test"
	lookupTimeout    = time.Second * 3
	publishTimeout   = time.Second * 3
)

func (s *Server) logError(ctx context.Context, err twirp.Error) context.Context {
	if err.Code() == twirp.FailedPrecondition {
		return ctx
	}
	delegation := rpc.GetDelegation(ctx)
	if delegation != nil {
		service, _ := twirp.ServiceName(ctx)
		method, _ := twirp.MethodName(ctx)
		l := s.logger.With(
			zap.String("remote", delegation.RemoteAddr().String()),
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

func (s *Server) attachRPC(ctx context.Context, router *transport.StreamRouter) {
	tunTwirp := protocol.NewTunnelServiceServer(s, twirp.WithServerHooks(&twirp.ServerHooks{
		RequestRouted: s.injectClientIdentity,
		Error:         s.logError,
	}))

	rpcHandler := chi.NewRouter()
	rpcHandler.Use(middleware.Recoverer)
	rpcHandler.Use(httprate.LimitByIP(10, time.Second))
	rpcHandler.Use(util.LimitBody(1 << 10)) // 1KB
	rpcHandler.Mount(tunTwirp.PathPrefix(), rpc.ExtractAuthorizationHeader(tunTwirp))

	srv := &http.Server{
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return rpc.WithDelegation(ctx, c.(*transport.StreamDelegate))
		},
		MaxHeaderBytes:    1 << 10, // 1KB
		ReadHeaderTimeout: time.Second * 3,
		Handler:           rpcHandler,
		ErrorLog:          util.GetStdLogger(s.logger, "rpc_server"),
	}

	go srv.Serve(s.rpcAcceptor)

	router.HandleTunnel(protocol.Stream_RPC, func(delegate *transport.StreamDelegate) {
		s.rpcAcceptor.Handle(delegate)
	})
}

func (s *Server) injectClientIdentity(ctx context.Context) (context.Context, error) {
	method, _ := twirp.MethodName(ctx)
	auth := rpc.GetAuthorization(ctx)

	switch method {
	case "RegisterIdentity":
		return ctx, nil
	case "Ping":
		// skip token check if not given
		// check if the client does provide one
		if len(auth) == 0 {
			return ctx, nil
		}
		fallthrough
	default:
		if len(auth) == 0 {
			return ctx, twirp.Unauthenticated.Error("encoded client token missing in authorization header")
		}
		token := &protocol.ClientToken{
			Token: []byte(auth),
		}
		verifiedClient, err := s.getClientByToken(ctx, token)
		if err != nil {
			return ctx, twirp.Unauthenticated.Errorf("failed to verify client token: %w", err)
		}

		ctx = rpc.WithClientToken(ctx, token)
		ctx = rpc.WithCientIdentity(ctx, verifiedClient)
		return ctx, nil
	}
}

func (s *Server) Ping(_ context.Context, _ *protocol.ClientPingRequest) (*protocol.ClientPingResponse, error) {
	return &protocol.ClientPingResponse{
		Node: s.tunnelTransport.Identity(),
		Apex: s.rootDomain,
	}, nil
}

func (s *Server) RegisterIdentity(ctx context.Context, req *protocol.RegisterIdentityRequest) (*protocol.RegisterIdentityResponse, error) {
	client := req.GetClient()
	if client == nil {
		return nil, twirp.InvalidArgument.Error("missing client identity in request")
	}
	delegation := rpc.GetDelegation(ctx)
	if delegation == nil {
		return nil, twirp.Internal.Error("delegation missing in context")
	}
	if delegation.Identity.GetId() != client.GetId() {
		return nil, twirp.PermissionDenied.Error("requesting client is not the same as connected client")
	}

	err := s.tunnelTransport.SendDatagram(client, []byte(testDatagramData))
	if err != nil {
		return nil, twirp.Aborted.Error("Client is not connected")
	}

	b, err := generateToken()
	if err != nil {
		return nil, twirp.Internal.Error(err.Error())
	}
	token := &protocol.ClientToken{
		Token: b,
	}

	if err := s.saveClientToken(ctx, token, client); err != nil {
		return nil, rpc.WrapErrorKV(tun.ClientTokenKey(token), err)
	}

	return &protocol.RegisterIdentityResponse{
		Token: token,
		Apex:  s.rootDomain,
	}, nil
}

func (s *Server) GetNodes(ctx context.Context, _ *protocol.GetNodesRequest) (*protocol.GetNodesResponse, error) {
	_, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	successors, err := s.chord.GetSuccessors()
	if err != nil {
		return nil, twirp.Internal.Error(err.Error())
	}

	vnodes := chord.MakeSuccListByAddress(s.chord, successors, tun.NumRedundantLinks)
	lookupJobs := make([]func(context.Context) (*protocol.Node, error), 0)
	for _, chord := range vnodes {
		if chord == nil {
			continue
		}
		chord := chord
		lookupJobs = append(lookupJobs, func(fnCtx context.Context) (*protocol.Node, error) {
			key := tun.DestinationByChordKey(chord.Identity())
			destination, err := s.lookupDestination(fnCtx, key)
			if err != nil {
				return nil, rpc.WrapErrorKV(key, err)
			}
			return destination.GetTunnel(), nil
		})
	}

	lookupCtx, lookupCancel := context.WithTimeout(ctx, lookupTimeout)
	defer lookupCancel()

	servers, errors := promise.All(lookupCtx, lookupJobs...)
	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	return &protocol.GetNodesResponse{
		Nodes: servers,
	}, nil
}

func (s *Server) GenerateHostname(ctx context.Context, req *protocol.GenerateHostnameRequest) (*protocol.GenerateHostnameResponse, error) {
	token, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	hostname := strings.Join(generator.MustGenerate(5), "-")
	prefix := tun.ClientHostnamesPrefix(token)
	if err := s.chord.PrefixAppend(ctx, []byte(prefix), []byte(hostname)); err != nil {
		return nil, rpc.WrapErrorKV(prefix, err)
	}

	return &protocol.GenerateHostnameResponse{
		Hostname: hostname,
	}, nil
}

func (s *Server) RegisteredHostnames(ctx context.Context, req *protocol.RegisteredHostnamesRequest) (*protocol.RegisteredHostnamesResponse, error) {
	token, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	prefix := tun.ClientHostnamesPrefix(token)

	children, err := s.chord.PrefixList(ctx, []byte(prefix))
	if err != nil {
		return nil, rpc.WrapErrorKV(prefix, err)
	}

	hostnames := make([]string, len(children))
	for i, child := range children {
		hostnames[i] = string(child)
	}

	return &protocol.RegisteredHostnamesResponse{
		Hostnames: hostnames,
	}, nil
}

func (s *Server) PublishTunnel(ctx context.Context, req *protocol.PublishTunnelRequest) (*protocol.PublishTunnelResponse, error) {
	token, verifiedClient, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	requested := uniqueNodes(req.GetServers())
	if len(requested) > tun.NumRedundantLinks {
		return nil, twirp.InvalidArgument.Error("too many requested endpoints")
	}
	if len(requested) < 1 {
		return nil, twirp.InvalidArgument.Error("no servers specified in request")
	}

	lease, err := s.chord.Acquire(ctx, []byte(tun.ClientLeaseKey(token)), time.Second*30)
	if err != nil {
		return nil, twirp.Internal.Errorf("error acquiring lease for publishing tunnel: %w", err)
	}
	defer s.chord.Release(ctx, []byte(tun.ClientLeaseKey(token)), lease)

	hostname := req.GetHostname()
	prefix := tun.ClientHostnamesPrefix(token)
	b, err := s.chord.PrefixContains(ctx, []byte(prefix), []byte(hostname))
	if err != nil {
		return nil, rpc.WrapErrorKV(prefix, err)
	}
	if !b {
		return nil, twirp.PermissionDenied.Errorf("hostname %s is not registered", hostname)
	}

	lookupJobs := make([]func(context.Context) (*protocol.TunnelDestination, error), len(requested))
	for i, server := range requested {
		key := tun.DestinationByTunnelKey(server)
		lookupJobs[i] = func(fnCtx context.Context) (*protocol.TunnelDestination, error) {
			destination, err := s.lookupDestination(fnCtx, key)
			if err != nil {
				return nil, rpc.WrapErrorKV(key, err)
			}
			return destination, nil
		}
	}

	lookupCtx, lookupCancel := context.WithTimeout(ctx, lookupTimeout)
	defer lookupCancel()

	destinations, errors := promise.All(lookupCtx, lookupJobs...)
	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	publishJobs := make([]func(context.Context) (*protocol.Node, error), len(destinations))
	for i, destination := range destinations {
		i := i
		dst := destination
		publishJobs[i] = func(fnCtx context.Context) (*protocol.Node, error) {
			bundle := &protocol.TunnelRoute{
				ClientDestination: verifiedClient,
				ChordDestination:  dst.GetChord(),
				TunnelDestination: dst.GetTunnel(),
				Hostname:          hostname,
			}
			val, err := bundle.MarshalVT()
			if err != nil {
				return nil, err
			}
			key := tun.RoutingKey(hostname, i+1)
			if err := s.chord.Put(fnCtx, []byte(key), val); err != nil {
				s.logger.Error("Error publishing route", zap.String("key", key), zap.Error(err))
				return nil, nil
			}
			return dst.GetTunnel(), nil
		}
	}

	publishCtx, publishCancel := context.WithTimeout(ctx, publishTimeout)
	defer publishCancel()

	maybePublished, errors := promise.All(publishCtx, publishJobs...)
	for _, err := range errors {
		if err != nil {
			return nil, twirp.InternalError(err.Error())
		}
	}
	// need to filter possible nil
	published := make([]*protocol.Node, 0)
	for _, node := range maybePublished {
		if node == nil {
			continue
		}
		published = append(published, node)
	}

	if len(published) == 0 {
		return nil, twirp.Unavailable.Error("unable to publish any routes")
	}

	return &protocol.PublishTunnelResponse{
		Published: published,
	}, nil
}

func (s *Server) UnpublishTunnel(ctx context.Context, req *protocol.UnpublishTunnelRequest) (*protocol.UnpublishTunnelResponse, error) {
	token, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	lease, err := s.chord.Acquire(ctx, []byte(tun.ClientLeaseKey(token)), time.Second*30)
	if err != nil {
		return nil, twirp.Internal.Errorf("error acquiring lease for unpublishing tunnel: %w", err)
	}
	defer s.chord.Release(ctx, []byte(tun.ClientLeaseKey(token)), lease)

	hostname := req.GetHostname()
	if err := s.unadvertiseTunnel(ctx, token, hostname); err != nil {
		return nil, err
	}

	return &protocol.UnpublishTunnelResponse{}, nil
}

func (s *Server) ReleaseTunnel(ctx context.Context, req *protocol.ReleaseTunnelRequest) (*protocol.ReleaseTunnelResponse, error) {
	token, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	lease, err := s.chord.Acquire(ctx, []byte(tun.ClientLeaseKey(token)), time.Second*30)
	if err != nil {
		return nil, twirp.Internal.Errorf("error acquiring lease for releasing tunnel: %w", err)
	}
	defer s.chord.Release(ctx, []byte(tun.ClientLeaseKey(token)), lease)

	hostname := req.GetHostname()
	if err := s.unadvertiseTunnel(ctx, token, hostname); err != nil {
		return nil, err
	}
	prefix := tun.ClientHostnamesPrefix(token)
	if err := s.chord.PrefixRemove(ctx, []byte(prefix), []byte(hostname)); err != nil {
		return nil, rpc.WrapErrorKV(prefix, err)
	}

	return &protocol.ReleaseTunnelResponse{}, nil
}

func (s *Server) unadvertiseTunnel(ctx context.Context, token *protocol.ClientToken, hostname string) error {
	prefix := tun.ClientHostnamesPrefix(token)
	b, err := s.chord.PrefixContains(ctx, []byte(prefix), []byte(hostname))
	if err != nil {
		return rpc.WrapErrorKV(prefix, err)
	}
	if !b {
		return twirp.PermissionDenied.Errorf("hostname %s is not registered", hostname)
	}

	unpublishJobs := make([]func(context.Context) (int, error), tun.NumRedundantLinks)
	for i := 0; i < tun.NumRedundantLinks; i++ {
		i := i
		unpublishJobs[i] = func(fnCtx context.Context) (int, error) {
			key := tun.RoutingKey(hostname, i+1)
			err := s.chord.Delete(fnCtx, []byte(key))
			return 0, err
		}
	}

	unpublishCtx, unpublishCancel := context.WithTimeout(ctx, publishTimeout)
	defer unpublishCancel()

	_, errors := promise.All(unpublishCtx, unpublishJobs...)
	for _, err := range errors {
		if err != nil {
			return twirp.InternalError(err.Error())
		}
	}

	return nil
}

func (s *Server) saveClientToken(ctx context.Context, token *protocol.ClientToken, client *protocol.Node) error {
	val, err := client.MarshalVT()
	if err != nil {
		return err
	}

	if err := s.chord.Put(ctx, []byte(tun.ClientTokenKey(token)), val); err != nil {
		return err
	}
	return nil
}

func (s *Server) getClientByToken(ctx context.Context, token *protocol.ClientToken) (*protocol.Node, error) {
	val, err := s.chord.Get(ctx, []byte(tun.ClientTokenKey(token)))
	if err != nil {
		return nil, err
	}
	if val == nil {
		return nil, fmt.Errorf("no existing client found with given token")
	}
	client := &protocol.Node{}
	if err := client.UnmarshalVT(val); err != nil {
		return nil, err
	}
	return client, nil
}

func extractAuthenticated(ctx context.Context) (*protocol.ClientToken, *protocol.Node, error) {
	token := rpc.GetClientToken(ctx)
	if token == nil {
		return nil, nil, twirp.Unauthenticated.Error("client is unauthenticated")
	}
	verifiedClient := rpc.GetClientIdentity(ctx)
	if verifiedClient == nil {
		return nil, nil, twirp.Unauthenticated.Error("client is unauthenticated")
	}
	delegation := rpc.GetDelegation(ctx)
	if delegation == nil {
		return nil, nil, twirp.Internal.Error("delegation missing in context")
	}
	if delegation.Identity.GetId() != verifiedClient.GetId() {
		return nil, nil, twirp.PermissionDenied.Error("requesting client is not the same as connected client")
	}
	return token, verifiedClient, nil
}

func uniqueNodes(nodes []*protocol.Node) []*protocol.Node {
	list := make([]*protocol.Node, 0)
	seen := make(map[string]bool)
	for _, node := range nodes {
		if node == nil || seen[node.GetAddress()] {
			continue
		}
		seen[node.GetAddress()] = true
		list = append(list, node)
	}
	return list
}

func generateToken() ([]byte, error) {
	b := make([]byte, 32)
	n, err := io.ReadFull(rand.Reader, b)
	if n != len(b) || err != nil {
		return nil, fmt.Errorf("error generating token")
	}
	return []byte(base64.StdEncoding.EncodeToString(b)), nil
}
