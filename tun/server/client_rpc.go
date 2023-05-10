package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"kon.nect.sh/specter/spec/acme"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/pki"
	"kon.nect.sh/specter/spec/pow"
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
		l := s.Logger.With(
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
		RequestRouted: s.verifyClientIdentity,
		Error:         s.logError,
	}))

	rpcHandler := chi.NewRouter()
	rpcHandler.Use(middleware.Recoverer)
	rpcHandler.Use(httprate.LimitByIP(10, time.Second))
	rpcHandler.Use(util.LimitBody(1 << 10)) // 1KB
	rpcHandler.Mount(tunTwirp.PathPrefix(), tunTwirp)

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
		ErrorLog:          util.GetStdLogger(s.Logger, "rpcServer"),
	}

	go srv.Serve(s.rpcAcceptor)

	router.HandleTunnel(protocol.Stream_RPC, func(delegate *transport.StreamDelegate) {
		s.rpcAcceptor.Handle(delegate)
	})
}

func (s *Server) verifyClientIdentity(ctx context.Context) (context.Context, error) {
	method, _ := twirp.MethodName(ctx)
	delegation := rpc.GetDelegation(ctx)
	if delegation == nil {
		return nil, twirp.Internal.Error("delegation missing in context")
	}

	switch method {
	case "Ping", "RegisterIdentity":
		return ctx, nil
	default:
		token, verifiedClient, err := extractAuthenticated(ctx)
		if err != nil {
			return ctx, err
		}
		cli, err := s.getClientByToken(ctx, token)
		if err != nil {
			return ctx, twirp.Unauthenticated.Errorf("failed to verify client token: %w", err)
		}
		// old format: before PKI
		if cli.GetAddress() == "" || !cli.GetRendezvous() {
			s.saveClientToken(ctx, token, verifiedClient)
		}
		return ctx, nil
	}
}

func (s *Server) Ping(_ context.Context, _ *protocol.ClientPingRequest) (*protocol.ClientPingResponse, error) {
	return &protocol.ClientPingResponse{
		Node: s.TunnelTransport.Identity(),
		Apex: s.Apex,
	}, nil
}

func (s *Server) RegisterIdentity(ctx context.Context, req *protocol.RegisterIdentityRequest) (*protocol.RegisterIdentityResponse, error) {
	token, verifiedClient, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	err = s.TunnelTransport.SendDatagram(verifiedClient, []byte(testDatagramData))
	if err != nil {
		return nil, twirp.Aborted.Error("client is not connected")
	}

	if err := s.saveClientToken(ctx, token, verifiedClient); err != nil {
		return nil, rpc.WrapErrorKV(tun.ClientTokenKey(token), err)
	}

	return &protocol.RegisterIdentityResponse{
		Apex: s.Apex,
	}, nil
}

func (s *Server) GetNodes(ctx context.Context, _ *protocol.GetNodesRequest) (*protocol.GetNodesResponse, error) {
	_, _, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	successors, err := s.Chord.GetSuccessors()
	if err != nil {
		return nil, twirp.Internal.Error(err.Error())
	}

	vnodes := chord.MakeSuccListByAddress(s.Chord, successors, tun.NumRedundantLinks)
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
	if err := s.Chord.PrefixAppend(ctx, []byte(prefix), []byte(hostname)); err != nil {
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

	children, err := s.Chord.PrefixList(ctx, []byte(prefix))
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

	lease, err := s.Chord.Acquire(ctx, []byte(tun.ClientLeaseKey(token)), time.Second*30)
	if err != nil {
		return nil, twirp.Internal.Errorf("error acquiring lease for publishing tunnel: %w", err)
	}
	defer s.Chord.Release(ctx, []byte(tun.ClientLeaseKey(token)), lease)

	hostname := req.GetHostname()
	prefix := tun.ClientHostnamesPrefix(token)
	b, err := s.Chord.PrefixContains(ctx, []byte(prefix), []byte(hostname))
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
			if err := s.Chord.Put(fnCtx, []byte(key), val); err != nil {
				s.Logger.Error("Error publishing route", zap.String("key", key), zap.Error(err))
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

	lease, err := s.Chord.Acquire(ctx, []byte(tun.ClientLeaseKey(token)), time.Second*30)
	if err != nil {
		return nil, twirp.Internal.Errorf("error acquiring lease for unpublishing tunnel: %w", err)
	}
	defer s.Chord.Release(ctx, []byte(tun.ClientLeaseKey(token)), lease)

	hostname := req.GetHostname()
	if err := s.unadvertiseTunnel(ctx, token, hostname); err != nil {
		return nil, err
	}

	return &protocol.UnpublishTunnelResponse{}, nil
}

func (s *Server) ReleaseTunnel(ctx context.Context, req *protocol.ReleaseTunnelRequest) (*protocol.ReleaseTunnelResponse, error) {
	token, client, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	lease, err := s.Chord.Acquire(ctx, []byte(tun.ClientLeaseKey(token)), time.Second*30)
	if err != nil {
		return nil, twirp.Internal.Errorf("error acquiring lease for releasing tunnel: %w", err)
	}
	defer s.Chord.Release(ctx, []byte(tun.ClientLeaseKey(token)), lease)

	hostname := req.GetHostname()
	if err := s.unadvertiseTunnel(ctx, token, hostname); err != nil {
		return nil, err
	}
	prefix := tun.ClientHostnamesPrefix(token)
	if err := s.Chord.PrefixRemove(ctx, []byte(prefix), []byte(hostname)); err != nil {
		return nil, rpc.WrapErrorKV(prefix, err)
	}

	if err := tun.RemoveCustomHostname(ctx, s.Chord, hostname); err != nil {
		s.Logger.Warn("Failed to remove custom hostname when releasing tunnel", zap.String("hostname", hostname), zap.Object("client", client), zap.Error(err))
	}

	return &protocol.ReleaseTunnelResponse{}, nil
}

func (s *Server) checkAcme(ctx context.Context, hostname string, proof *protocol.ProofOfWork, token *protocol.ClientToken, client *protocol.Node) (found bool, err error) {
	_, err = pow.VerifySolution(proof, pow.Parameters{
		Difficulty: acme.HashcashDifficulty,
		Expires:    acme.HashcashExpires,
		GetSubject: func(pubKey ed25519.PublicKey) string {
			return hostname
		},
	})
	if err != nil {
		return false, err
	}

	if strings.Contains(hostname, s.Acme) || strings.Contains(hostname, s.Apex) {
		return false, twirp.InvalidArgumentError("hostname", "provided hostname is not valid for custom hostname")
	}

	bundle, err := tun.FindCustomHostname(ctx, s.Chord, hostname)
	switch err {
	case nil:
		// alreday exist
		if !bytes.Equal(bundle.GetClientToken().GetToken(), token.GetToken()) ||
			bundle.GetClientIdentity().GetId() != client.GetId() ||
			bundle.GetClientIdentity().GetAddress() != client.GetAddress() {
			return false, twirp.AlreadyExists.Error("hostname is already registered")
		}
		return true, nil
	case tun.ErrHostnameNotFound:
		// expected
		return false, nil
	default:
		return false, err
	}
}

func (s *Server) AcmeInstruction(ctx context.Context, req *protocol.InstructionRequest) (*protocol.InstructionResponse, error) {
	token, client, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	hostname, err := acme.Normalize(req.GetHostname())
	if err != nil {
		return nil, twirp.InvalidArgumentError("hostname", err.Error())
	}

	if _, err := s.checkAcme(ctx, hostname, req.GetProof(), token, client); err != nil {
		return nil, err
	}

	name, content := acme.GenerateRecord(hostname, s.Acme, token.GetToken())
	return &protocol.InstructionResponse{
		Name:    name,
		Content: content,
	}, nil
}

func (s *Server) AcmeValidate(ctx context.Context, req *protocol.ValidateRequest) (*protocol.ValidateResponse, error) {
	token, client, err := extractAuthenticated(ctx)
	if err != nil {
		return nil, err
	}

	hostname, err := acme.Normalize(req.GetHostname())
	if err != nil {
		return nil, twirp.InvalidArgumentError("hostname", err.Error())
	}

	found, err := s.checkAcme(ctx, hostname, req.GetProof(), token, client)
	if err != nil {
		return nil, err
	}

	var (
		start time.Time
		cname string
	)

	name, content := acme.GenerateRecord(hostname, s.Acme, token.GetToken())

	lookupCtx, lookupCancel := context.WithTimeout(ctx, lookupTimeout)
	defer lookupCancel()

	if found {
		goto VALIDATED
	}

	start = time.Now()
	cname, err = s.Resolver.LookupCNAME(lookupCtx, name)
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}

	if cname != content {
		return nil, twirp.FailedPrecondition.Errorf("unexpected CNAME content: %s", cname)
	}

	s.Logger.Info("Custom hostname validated", zap.String("hostname", hostname), zap.Duration("took", time.Since(start)), zap.Object("client", client))

VALIDATED:
	if err := tun.SaveCustomHostname(ctx, s.Chord, hostname, &protocol.CustomHostname{
		ClientIdentity: client,
		ClientToken:    token,
	}); err != nil {
		return nil, twirp.InternalErrorWith(err)
	}

	prefix := tun.ClientHostnamesPrefix(token)
	err = s.Chord.PrefixAppend(ctx, []byte(prefix), []byte(hostname))
	if err != nil && !errors.Is(err, chord.ErrKVPrefixConflict) {
		return nil, twirp.InternalErrorWith(err)
	}

	return &protocol.ValidateResponse{
		Apex: s.Apex,
	}, nil
}

func (s *Server) unadvertiseTunnel(ctx context.Context, token *protocol.ClientToken, hostname string) error {
	prefix := tun.ClientHostnamesPrefix(token)
	b, err := s.Chord.PrefixContains(ctx, []byte(prefix), []byte(hostname))
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
			err := s.Chord.Delete(fnCtx, []byte(key))
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

	if err := s.Chord.Put(ctx, []byte(tun.ClientTokenKey(token)), val); err != nil {
		return err
	}
	return nil
}

func (s *Server) getClientByToken(ctx context.Context, token *protocol.ClientToken) (*protocol.Node, error) {
	val, err := s.Chord.Get(ctx, []byte(tun.ClientTokenKey(token)))
	if err != nil {
		return nil, err
	}
	if len(val) == 0 {
		return nil, fmt.Errorf("no existing client found with given token")
	}
	client := &protocol.Node{}
	if err := client.UnmarshalVT(val); err != nil {
		return nil, err
	}
	return client, nil
}

func extractAuthenticated(ctx context.Context) (*protocol.ClientToken, *protocol.Node, error) {
	delegation := rpc.GetDelegation(ctx)
	if delegation == nil {
		return nil, nil, twirp.Internal.Error("delegation missing in context")
	}
	if delegation.Certificate == nil {
		return nil, nil, twirp.Unauthenticated.Error("missing client certificate")
	}
	identity, err := pki.ExtractCertificateIdentity(delegation.Certificate)
	if err != nil {
		return nil, nil, twirp.Unauthenticated.Error(err.Error())
	}
	return &protocol.ClientToken{
		Token: identity.Token,
	}, identity.NodeIdentity(), nil
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
