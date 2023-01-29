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
	"kon.nect.sh/specter/util/httprate"
	"kon.nect.sh/specter/util/router"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sethvargo/go-diceware/diceware"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
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
		s.logger.Error("Error handling RPC request",
			zap.String("remote", delegation.RemoteAddr().String()),
			zap.Uint64("peer", delegation.Identity.GetId()),
			zap.String("service", service),
			zap.String("method", method),
			zap.Error(err),
		)
	}
	return ctx
}

func (s *Server) attachRPC(ctx context.Context, router *router.StreamRouter) {
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
		encoded, err := base64.StdEncoding.DecodeString(auth)
		if err != nil {
			return ctx, twirp.Unauthenticated.Errorf("cannot decode token: %w", err)
		}
		token := &protocol.ClientToken{
			Token: encoded,
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
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}

	return &protocol.RegisterIdentityResponse{
		Token: token,
		Apex:  s.rootDomain,
	}, nil
}

func (s *Server) GetNodes(ctx context.Context, _ *protocol.GetNodesRequest) (*protocol.GetNodesResponse, error) {
	_, err := verifyIdentity(ctx)
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
		identities, err := s.lookupIdentities(ctx, tun.IdentitiesChordKey(chord.Identity()))
		if err != nil {
			return nil, twirp.FailedPrecondition.Error(err.Error())
		}
		servers = append(servers, identities.GetTun())
	}

	return &protocol.GetNodesResponse{
		Nodes: servers,
	}, nil
}

func (s *Server) GenerateHostname(ctx context.Context, req *protocol.GenerateHostnameRequest) (*protocol.GenerateHostnameResponse, error) {
	token := rpc.GetClientToken(ctx)
	if token == nil {
		return nil, twirp.Unauthenticated.Error("client is unauthenticated")
	}
	_, err := verifyIdentity(ctx)
	if err != nil {
		return nil, err
	}

	hostname := strings.Join(generator.MustGenerate(5), "-")
	if err := s.chord.PrefixAppend(ctx, []byte(tun.ClientHostnamesPrefix(token)), []byte(hostname)); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}

	return &protocol.GenerateHostnameResponse{
		Hostname: hostname,
	}, nil
}

func (s *Server) PublishTunnel(ctx context.Context, req *protocol.PublishTunnelRequest) (*protocol.PublishTunnelResponse, error) {
	token := rpc.GetClientToken(ctx)
	if token == nil {
		return nil, twirp.Unauthenticated.Error("client is unauthenticated")
	}
	verifiedClient, err := verifyIdentity(ctx)
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
	b, err := s.chord.PrefixContains(ctx, []byte(tun.ClientHostnamesPrefix(token)), []byte(hostname))
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	if !b {
		return nil, twirp.PermissionDenied.Errorf("hostname %s is not registered", hostname)
	}

	identities := make([]*protocol.IdentitiesPair, len(requested))
	for k, server := range requested {
		identity, err := s.lookupIdentities(ctx, tun.IdentitiesTunnelKey(server))
		if err != nil {
			return nil, twirp.FailedPrecondition.Error(err.Error())
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

func verifyIdentity(ctx context.Context) (*protocol.Node, error) {
	verifiedClient := rpc.GetClientIdentity(ctx)
	if verifiedClient == nil {
		return nil, twirp.Unauthenticated.Error("client is unauthenticated")
	}
	delegation := rpc.GetDelegation(ctx)
	if delegation == nil {
		return nil, twirp.Internal.Error("delegation missing in context")
	}
	if delegation.Identity.GetId() != verifiedClient.GetId() {
		return nil, twirp.PermissionDenied.Error("requesting client is not the same as connected client")
	}
	return verifiedClient, nil
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
