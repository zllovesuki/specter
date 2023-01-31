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
)

func (s *Server) logError(ctx context.Context, err twirp.Error) context.Context {
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
		MaxHeaderBytes: 1 << 10, // 1KB
		ReadTimeout:    time.Second * 3,
		Handler:        rpcHandler,
		ErrorLog:       util.GetStdLogger(s.logger, "rpc_server"),
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
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}

	vnodes := chord.MakeSuccList(s.chord, successors, tun.NumRedundantLinks)

	servers := make([]*protocol.Node, 0)
	for _, chord := range vnodes {
		if chord == nil {
			continue
		}
		key := tun.IdentitiesChordKey(chord.Identity())
		identities, err := s.lookupIdentities(ctx, key)
		if err != nil {
			return nil, rpc.WrapErrorKV(key, err)
		}
		servers = append(servers, identities.GetTun())
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
		return nil, twirp.FailedPrecondition.Errorf("error acquiring token: %w", err)
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

	identities := make([]*protocol.IdentitiesPair, len(requested))
	for k, server := range requested {
		key := tun.IdentitiesTunnelKey(server)
		identity, err := s.lookupIdentities(ctx, key)
		if err != nil {
			return nil, rpc.WrapErrorKV(key, err)
		}
		identities[k] = identity
	}

	published := make([]*protocol.Node, 0)
	for k, identity := range identities {
		bundle := &protocol.Tunnel{
			Client:   verifiedClient,
			Chord:    identity.GetChord(),
			Tun:      identity.GetTun(),
			Hostname: hostname,
		}
		val, err := bundle.MarshalVT()
		if err != nil {
			return nil, twirp.Internal.Error(err.Error())
		}
		key := tun.RoutingKey(hostname, k+1)
		if err := s.chord.Put(ctx, []byte(key), val); err != nil {
			continue
		}
		published = append(published, identity.GetTun())
	}

	return &protocol.PublishTunnelResponse{
		Published: published,
	}, nil
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
	seen := make(map[uint64]bool)
	for _, node := range nodes {
		if node == nil || seen[node.GetId()] {
			continue
		}
		seen[node.GetId()] = true
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
