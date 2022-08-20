package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"kon.nect.sh/specter/rpc"
	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	rpcSpec "kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/tun"

	"github.com/sethvargo/go-diceware/diceware"
	"go.uber.org/zap"
)

var generator, _ = diceware.NewGenerator(nil)

const (
	testDatagramData = "test"
)

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

func (s *Server) HandleRPC(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case delegate := <-s.clientTransport.RPC():
			conn := delegate.Connection
			l := s.logger.With(
				zap.Any("peer", delegate.Identity),
				zap.String("remote", conn.RemoteAddr().String()),
				zap.String("local", conn.LocalAddr().String()))
			r := rpc.NewRPC(
				l.With(zap.String("pov", "client_rpc")),
				conn,
				s.rpcHandlerMiddlerware(delegate.Identity))
			go r.Start(ctx)
		}
	}
}

func (s *Server) saveClientToken(token *protocol.ClientToken, client *protocol.Node) error {
	val, err := client.MarshalVT()
	if err != nil {
		return err
	}

	if err := s.chord.Put([]byte(tun.ClientTokenKey(token)), val); err != nil {
		return err
	}
	return nil
}

func (s *Server) getClientByToken(token *protocol.ClientToken) (*protocol.Node, error) {
	val, err := s.chord.Get([]byte(tun.ClientTokenKey(token)))
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

func (s *Server) rpcHandlerMiddlerware(client *protocol.Node) rpcSpec.RPCHandler {
	return func(ctx context.Context, req *protocol.RPC_Request) (*protocol.RPC_Response, error) {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("server has shutdown")
		default:
		}

		if req.GetKind() != protocol.RPC_CLIENT_REQUEST {
			return nil, fmt.Errorf("unknown RPC call: %s", req.GetKind())
		}

		var verifiedClient *protocol.Node
		var err error

		tReq := req.GetClientRequest()
		switch tReq.GetKind() {
		case protocol.TunnelRPC_IDENTITY:
			if tReq.GetRegisterRequest().GetClient() == nil {
				return nil, fmt.Errorf("missing client")
			}
			if client.GetId() != tReq.GetRegisterRequest().GetClient().GetId() {
				return nil, fmt.Errorf("registration client is not the same as connected client")
			}
		case protocol.TunnelRPC_PING:
			// skip token check if not specified
			// check if the client does provide one
			if tReq.GetToken() == nil {
				break
			}
			fallthrough
		default:
			token := tReq.GetToken()
			if token == nil {
				return nil, fmt.Errorf("missing token")
			}
			verifiedClient, err = s.getClientByToken(token)
			if err != nil {
				return nil, err
			}
		}

		tResp, err := s.rpcHandler(ctx, verifiedClient, tReq)
		if err != nil {
			return nil, err
		}
		return &protocol.RPC_Response{
			ClientResponse: tResp,
		}, nil
	}
}

func (s *Server) rpcHandler(ctx context.Context, verifiedClient *protocol.Node, req *protocol.ClientRequest) (*protocol.ClientResponse, error) {
	resp := &protocol.ClientResponse{}

	switch req.GetKind() {
	case protocol.TunnelRPC_PING:
		resp.PingResponse = &protocol.ClientPingResponse{
			Node: s.clientTransport.Identity(),
			Apex: s.rootDomain,
		}

	case protocol.TunnelRPC_IDENTITY:
		client := req.GetRegisterRequest().GetClient()

		err := s.clientTransport.SendDatagram(client, []byte(testDatagramData))
		if err != nil {
			return nil, fmt.Errorf("client not connected")
		}

		b, err := generateToken()
		if err != nil {
			return nil, err
		}
		token := &protocol.ClientToken{
			Token: b,
		}

		if err := s.saveClientToken(token, client); err != nil {
			return nil, err
		}

		resp.RegisterResponse = &protocol.RegisterIdentityResponse{
			Token: token,
			Apex:  s.rootDomain,
		}

	case protocol.TunnelRPC_NODES:
		successors, err := s.chord.GetSuccessors()
		if err != nil {
			return nil, err
		}

		vnodes := chord.MakeSuccList(s.chord, successors, tun.NumRedundantLinks)

		servers := make([]*protocol.Node, 0)
		for _, chord := range vnodes {
			if chord == nil {
				continue
			}
			identities, err := s.lookupIdentities(tun.IdentitiesChordKey(chord.Identity()))
			if err != nil {
				return nil, err
			}
			servers = append(servers, identities.GetTun())
		}
		resp.NodesResponse = &protocol.GetNodesResponse{
			Nodes: servers,
		}

	case protocol.TunnelRPC_HOSTNAME:
		token := req.GetToken()

		hostname := strings.Join(generator.MustGenerate(5), "-")
		if err := s.chord.PrefixAppend([]byte(tun.ClientHostnamesPrefix(token)), []byte(hostname)); err != nil {
			return nil, err
		}

		resp.HostnameResponse = &protocol.GenerateHostnameResponse{
			Hostname: hostname,
		}

	case protocol.TunnelRPC_TUNNEL:
		token := req.GetToken()

		requested := uniqueNodes(req.GetTunnelRequest().GetServers())
		if len(requested) > tun.NumRedundantLinks {
			return nil, fmt.Errorf("too many requested endpoints")
		}
		if len(requested) < 1 {
			return nil, fmt.Errorf("no servers specified in request")
		}

		lease, err := s.chord.Acquire([]byte(tun.ClientLeaseKey(token)), time.Second*30)
		if err != nil {
			return nil, fmt.Errorf("error acquiring token: %w", err)
		}
		defer s.chord.Release([]byte(tun.ClientLeaseKey(token)), lease)

		hostname := req.GetTunnelRequest().GetHostname()
		b, err := s.chord.PrefixContains([]byte(tun.ClientHostnamesPrefix(token)), []byte(hostname))
		if err != nil {
			return nil, err
		}
		if !b {
			return nil, fmt.Errorf("hostname %s is not registered", hostname)
		}

		identities := make([]*protocol.IdentitiesPair, len(requested))
		for k, server := range requested {
			identity, err := s.lookupIdentities(tun.IdentitiesTunKey(server))
			if err != nil {
				return nil, err
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
				return nil, err
			}
			key := tun.RoutingKey(hostname, k+1)
			if err := s.chord.Put([]byte(key), val); err != nil {
				continue
			}
			published = append(published, identity.GetTun())
		}
		resp.TunnelResponse = &protocol.PublishTunnelResponse{
			Published: published,
		}

	default:
		s.logger.Warn("Unknown RPC Call", zap.String("kind", req.GetKind().String()))
		return nil, fmt.Errorf("unknown RPC call: %s", req.GetKind())
	}

	return resp, nil
}
