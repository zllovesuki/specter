package chord

import (
	"context"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/spec/rpc"
)

type Server struct {
	LocalNode chord.VNode
	Factory   RemoteNodeFactory
}

type RemoteNodeFactory func(*protocol.Node) (chord.VNode, error)

var _ protocol.KVService = (*Server)(nil)
var _ protocol.VNodeService = (*Server)(nil)

func (r *Server) Identity(_ context.Context, _ *protocol.IdentityRequest) (*protocol.IdentityResponse, error) {
	return &protocol.IdentityResponse{
		Identity: r.LocalNode.Identity(),
	}, nil
}

func (r *Server) Ping(_ context.Context, _ *protocol.PingRequest) (*protocol.PingResponse, error) {
	if err := r.LocalNode.Ping(); err != nil {
		return nil, rpc.WrapError(err)
	}
	return &protocol.PingResponse{}, nil
}

func (r *Server) Notify(_ context.Context, req *protocol.NotifyRequest) (*protocol.NotifyResponse, error) {
	predecessor := req.GetPredecessor()

	vnode, err := r.Factory(predecessor)
	if err != nil {
		return nil, rpc.WrapError(err)
	}
	err = r.LocalNode.Notify(vnode)
	if err != nil {
		return nil, rpc.WrapError(err)
	}

	return &protocol.NotifyResponse{}, nil
}

func (r *Server) FindSuccessor(_ context.Context, req *protocol.FindSuccessorRequest) (*protocol.FindSuccessorResponse, error) {
	key := req.GetKey()
	vnode, err := r.LocalNode.FindSuccessor(key)
	if err != nil {
		return nil, rpc.WrapError(err)
	}
	return &protocol.FindSuccessorResponse{
		Successor: vnode.Identity(),
	}, nil
}

func (r *Server) GetSuccessors(_ context.Context, _ *protocol.GetSuccessorsRequest) (*protocol.GetSuccessorsResponse, error) {
	vnodes, err := r.LocalNode.GetSuccessors()
	if err != nil {
		return nil, rpc.WrapError(err)
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

func (r *Server) GetPredecessor(_ context.Context, _ *protocol.GetPredecessorRequest) (*protocol.GetPredecessorResponse, error) {
	vnode, err := r.LocalNode.GetPredecessor()
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

func (r *Server) RequestToJoin(_ context.Context, req *protocol.RequestToJoinRequest) (*protocol.RequestToJoinResponse, error) {
	joiner := req.GetJoiner()

	vnode, err := r.Factory(joiner)
	if err != nil {
		return nil, rpc.WrapError(err)
	}

	pre, vnodes, err := r.LocalNode.RequestToJoin(vnode)
	if err != nil {
		return nil, rpc.WrapError(err)
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

func (r *Server) FinishJoin(_ context.Context, req *protocol.MembershipConclusionRequest) (*protocol.MembershipConclusionResponse, error) {
	if err := r.LocalNode.FinishJoin(req.GetStabilize(), req.GetRelease()); err != nil {
		return nil, rpc.WrapError(err)
	}
	return &protocol.MembershipConclusionResponse{}, nil
}

func (r *Server) RequestToLeave(_ context.Context, req *protocol.RequestToLeaveRequest) (*protocol.RequestToLeaveResponse, error) {
	leaver := req.GetLeaver()

	vnode, err := r.Factory(leaver)
	if err != nil {
		return nil, rpc.WrapError(err)
	}

	if err := r.LocalNode.RequestToLeave(vnode); err != nil {
		return nil, rpc.WrapError(err)
	}
	return &protocol.RequestToLeaveResponse{}, nil
}

func (r *Server) FinishLeave(_ context.Context, req *protocol.MembershipConclusionRequest) (*protocol.MembershipConclusionResponse, error) {
	if err := r.LocalNode.FinishLeave(req.GetStabilize(), req.GetRelease()); err != nil {
		return nil, rpc.WrapError(err)
	}
	return &protocol.MembershipConclusionResponse{}, nil
}

func (r *Server) Put(ctx context.Context, req *protocol.SimpleRequest) (*protocol.SimpleResponse, error) {
	if err := r.LocalNode.Put(ctx, req.GetKey(), req.GetValue()); err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetKey()), err)
	}
	return &protocol.SimpleResponse{}, nil
}

func (r *Server) Get(ctx context.Context, req *protocol.SimpleRequest) (*protocol.SimpleResponse, error) {
	val, err := r.LocalNode.Get(ctx, req.GetKey())
	if err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetKey()), err)
	}
	return &protocol.SimpleResponse{
		Value: val,
	}, nil
}

func (r *Server) Delete(ctx context.Context, req *protocol.SimpleRequest) (*protocol.SimpleResponse, error) {
	err := r.LocalNode.Delete(ctx, req.GetKey())
	if err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetKey()), err)
	}
	return &protocol.SimpleResponse{}, nil
}

func (r *Server) Append(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	if err := r.LocalNode.PrefixAppend(ctx, req.GetPrefix(), req.GetChild()); err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetPrefix()), err)
	}
	return &protocol.PrefixResponse{}, nil
}

func (r *Server) List(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	children, err := r.LocalNode.PrefixList(ctx, req.GetPrefix())
	if err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetPrefix()), err)
	}
	return &protocol.PrefixResponse{
		Children: children,
	}, nil
}

func (r *Server) Contains(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	exists, err := r.LocalNode.PrefixContains(ctx, req.GetPrefix(), req.GetChild())
	if err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetPrefix()), err)
	}
	return &protocol.PrefixResponse{
		Exists: exists,
	}, nil
}

func (r *Server) Remove(ctx context.Context, req *protocol.PrefixRequest) (*protocol.PrefixResponse, error) {
	if err := r.LocalNode.PrefixRemove(ctx, req.GetPrefix(), req.GetChild()); err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetPrefix()), err)
	}
	return &protocol.PrefixResponse{}, nil
}

func (r *Server) Acquire(ctx context.Context, req *protocol.LeaseRequest) (*protocol.LeaseResponse, error) {
	token, err := r.LocalNode.Acquire(ctx, req.GetLease(), req.GetTtl().AsDuration())
	if err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetLease()), err)
	}
	return &protocol.LeaseResponse{
		Token: token,
	}, nil
}

func (r *Server) Renew(ctx context.Context, req *protocol.LeaseRequest) (*protocol.LeaseResponse, error) {
	token, err := r.LocalNode.Renew(ctx, req.GetLease(), req.GetTtl().AsDuration(), req.GetPrevToken())
	if err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetLease()), err)
	}
	return &protocol.LeaseResponse{
		Token: token,
	}, nil
}

func (r *Server) Release(ctx context.Context, req *protocol.LeaseRequest) (*protocol.LeaseResponse, error) {
	if err := r.LocalNode.Release(ctx, req.GetLease(), req.GetPrevToken()); err != nil {
		return nil, rpc.WrapErrorKV(string(req.GetLease()), err)
	}
	return &protocol.LeaseResponse{}, nil
}

func (r *Server) Import(ctx context.Context, req *protocol.ImportRequest) (*protocol.ImportResponse, error) {
	if err := r.LocalNode.Import(ctx, req.GetKeys(), req.GetValues()); err != nil {
		return nil, rpc.WrapError(err)
	}
	return &protocol.ImportResponse{}, nil
}
