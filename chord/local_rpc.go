package chord

import (
	"context"
	"net"
	"net/http"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
	"kon.nect.sh/specter/spec/transport"
	"kon.nect.sh/specter/util"
	"kon.nect.sh/specter/util/ratecounter"
	"kon.nect.sh/specter/util/router"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/twitchtv/twirp"
	"go.uber.org/zap"
)

func (n *LocalNode) logError(ctx context.Context, err twirp.Error) context.Context {
	delegation := rpc.GetDelegation(ctx)
	if delegation != nil {
		service, _ := twirp.ServiceName(ctx)
		method, _ := twirp.MethodName(ctx)
		n.Logger.Error("Error handling RPC request",
			zap.Uint64("peer", delegation.Identity.GetId()),
			zap.String("service", service),
			zap.String("method", method),
			zap.Error(err),
		)
	}
	return ctx
}

func (n *LocalNode) getIncrementor(rate *ratecounter.Rate) func(ctx context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		rate.Increment()
		return ctx, nil
	}
}

func (n *LocalNode) AttachRouter(ctx context.Context, router *router.StreamRouter) {
	n.stopWg.Add(1)

	r := &rpcServer{
		baseContext: ctx,
		logger:      n.Logger,
		local:       n,
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
	rpcHandler.Mount(nsTwirp.PathPrefix(), rpc.ExtractSerializedContext(nsTwirp))
	rpcHandler.Mount(ksTwirp.PathPrefix(), rpc.ExtractSerializedContext(ksTwirp))

	srv := &http.Server{
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return rpc.WithDelegation(ctx, c.(*transport.StreamDelegate))
		},
		ReadTimeout: rpcTimeout,
		Handler:     rpcHandler,
		ErrorLog:    util.GetStdLogger(n.Logger, "rpc_server"),
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

type rpcServer struct {
	baseContext context.Context
	logger      *zap.Logger
	local       *LocalNode
}

var _ protocol.KVService = (*rpcServer)(nil)
var _ protocol.VNodeService = (*rpcServer)(nil)

func (r *rpcServer) Identity(_ context.Context, _ *protocol.IdentityRequest) (*protocol.IdentityResponse, error) {
	return &protocol.IdentityResponse{
		Identity: r.local.Identity(),
	}, nil
}

func (r *rpcServer) Ping(_ context.Context, _ *protocol.PingRequest) (*protocol.PingResponse, error) {
	if err := r.local.Ping(); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.PingResponse{}, nil
}

func (r *rpcServer) Notify(_ context.Context, req *protocol.NotifyRequest) (*protocol.NotifyResponse, error) {
	predecessor := req.GetPredecessor()

	vnode, err := createRPC(r.baseContext, r.logger, r.local.ChordClient, predecessor)
	if err != nil {
		return nil, twirp.Internal.Error(err.Error())
	}

	err = r.local.Notify(vnode)
	if err != nil {
		return nil, twirp.Internal.Error(err.Error())
	}

	return &protocol.NotifyResponse{}, nil
}

func (r *rpcServer) FindSuccessor(_ context.Context, req *protocol.FindSuccessorRequest) (*protocol.FindSuccessorResponse, error) {
	key := req.GetKey()
	vnode, err := r.local.FindSuccessor(key)
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.FindSuccessorResponse{
		Successor: vnode.Identity(),
	}, nil
}

func (r *rpcServer) GetSuccessors(_ context.Context, _ *protocol.GetSuccessorsRequest) (*protocol.GetSuccessorsResponse, error) {
	vnodes, err := r.local.GetSuccessors()
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	identities := make([]*protocol.Node, 0)
	for _, vnode := range vnodes {
		if vnode == nil {
			continue
		}
		identities = append(identities, vnode.Identity())
	}
	return &protocol.GetSuccessorsResponse{
		Successors: identities,
	}, nil
}

func (r *rpcServer) GetPredecessor(_ context.Context, _ *protocol.GetPredecessorRequest) (*protocol.GetPredecessorResponse, error) {
	vnode, err := r.local.GetPredecessor()
	if err != nil {
		return nil, err
	}
	var pre *protocol.Node
	if vnode != nil {
		pre = vnode.Identity()
	}
	return &protocol.GetPredecessorResponse{
		Predecessor: pre,
	}, nil
}

func (r *rpcServer) RequestToJoin(_ context.Context, req *protocol.RequestToJoinRequest) (*protocol.RequestToJoinResponse, error) {
	joiner := req.GetJoiner()
	vnode, err := createRPC(r.baseContext, r.logger, r.local.ChordClient, joiner)
	if err != nil {
		return nil, twirp.Internal.Error(err.Error())
	}

	pre, vnodes, err := r.local.RequestToJoin(vnode)
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}

	successors := make([]*protocol.Node, 0)
	for _, vnode := range vnodes {
		if vnode == nil {
			continue
		}
		successors = append(successors, vnode.Identity())
	}

	return &protocol.RequestToJoinResponse{
		Predecessor: pre.Identity(),
		Successors:  successors,
	}, nil
}

func (r *rpcServer) FinishJoin(_ context.Context, req *protocol.MembershipConclusionRequest) (*protocol.MembershipConclusionResponse, error) {
	if err := r.local.FinishJoin(req.GetStablize(), req.GetRelease()); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.MembershipConclusionResponse{}, nil
}

func (r *rpcServer) RequestToLeave(_ context.Context, req *protocol.RequestToLeaveRequest) (*protocol.RequestToLeaveResponse, error) {
	leaver := req.GetLeaver()
	vnode, err := createRPC(r.baseContext, r.logger, r.local.ChordClient, leaver)
	if err != nil {
		return nil, twirp.Internal.Error(err.Error())
	}
	if err := r.local.RequestToLeave(vnode); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.RequestToLeaveResponse{}, nil
}

func (r *rpcServer) FinishLeave(_ context.Context, req *protocol.MembershipConclusionRequest) (*protocol.MembershipConclusionResponse, error) {
	if err := r.local.FinishLeave(req.GetStablize(), req.GetRelease()); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.MembershipConclusionResponse{}, nil
}

func (r *rpcServer) Put(ctx context.Context, req *protocol.SimpleRequest) (*protocol.SimpleResponse, error) {
	if err := r.local.Put(ctx, req.GetKey(), req.GetValue()); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.SimpleResponse{}, nil
}

func (r *rpcServer) Get(ctx context.Context, req *protocol.SimpleRequest) (*protocol.SimpleResponse, error) {
	val, err := r.local.Get(ctx, req.GetKey())
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.SimpleResponse{
		Value: val,
	}, nil
}

func (r *rpcServer) Delete(ctx context.Context, req *protocol.SimpleRequest) (*protocol.SimpleResponse, error) {
	err := r.local.Delete(ctx, req.GetKey())
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.SimpleResponse{}, nil
}

func (r *rpcServer) Append(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	if err := r.local.PrefixAppend(ctx, req.GetPrefix(), req.GetChild()); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.PrefixResponse{}, nil
}

func (r *rpcServer) List(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	children, err := r.local.PrefixList(ctx, req.GetPrefix())
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.PrefixResponse{
		Children: children,
	}, nil
}

func (r *rpcServer) Contains(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	exists, err := r.local.PrefixContains(ctx, req.GetPrefix(), req.GetChild())
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.PrefixResponse{
		Exists: exists,
	}, nil
}

func (r *rpcServer) Remove(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	if err := r.local.PrefixRemove(ctx, req.GetPrefix(), req.GetChild()); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.PrefixResponse{}, nil
}

func (r *rpcServer) Acquire(ctx context.Context, req *protocol.LeaseRequest) (*protocol.LeaseResponse, error) {
	token, err := r.local.Acquire(ctx, req.GetLease(), req.GetTtl().AsDuration())
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.LeaseResponse{
		Token: token,
	}, nil
}

func (r *rpcServer) Renew(ctx context.Context, req *protocol.LeaseRequest) (*protocol.LeaseResponse, error) {
	token, err := r.local.Renew(ctx, req.GetLease(), req.GetTtl().AsDuration(), req.GetPrevToken())
	if err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.LeaseResponse{
		Token: token,
	}, nil
}

func (r *rpcServer) Release(ctx context.Context, req *protocol.LeaseRequest) (*protocol.LeaseResponse, error) {
	if err := r.local.Release(ctx, req.GetLease(), req.GetPrevToken()); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.LeaseResponse{}, nil
}

func (r *rpcServer) Import(ctx context.Context, req *protocol.ImportRequest) (*protocol.ImportResponse, error) {
	if err := r.local.Import(ctx, req.GetKeys(), req.GetValues()); err != nil {
		return nil, twirp.FailedPrecondition.Error(err.Error())
	}
	return &protocol.ImportResponse{}, nil
}
