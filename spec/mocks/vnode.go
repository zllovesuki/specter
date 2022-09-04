//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"context"
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
)

type VNode struct {
	mock.Mock
}

var _ chord.VNode = (*VNode)(nil)

func (n *VNode) Put(ctx context.Context, key []byte, value []byte) error {
	args := n.Called(ctx, key, value)
	return args.Error(0)
}

func (n *VNode) Get(ctx context.Context, key []byte) (value []byte, err error) {
	args := n.Called(ctx, key)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.([]byte), e
}

func (n *VNode) Delete(ctx context.Context, key []byte) error {
	args := n.Called(ctx, key)
	return args.Error(0)
}

func (n *VNode) PrefixAppend(ctx context.Context, prefix []byte, child []byte) error {
	args := n.Called(ctx, prefix, child)
	e := args.Error(0)
	return e
}

func (n *VNode) PrefixList(ctx context.Context, prefix []byte) ([][]byte, error) {
	args := n.Called(ctx, prefix)
	v := args.Get(0)
	e := args.Error(1)
	return v.([][]byte), e
}

func (n *VNode) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	args := n.Called(ctx, prefix, child)
	v := args.Bool(0)
	e := args.Error(1)
	return v, e
}

func (n *VNode) PrefixRemove(ctx context.Context, prefix []byte, child []byte) error {
	args := n.Called(ctx, prefix, child)
	e := args.Error(0)
	return e
}

func (n *VNode) Acquire(ctx context.Context, lease []byte, ttl time.Duration) (token uint64, err error) {
	args := n.Called(ctx, lease, ttl)
	t := args.Get(0)
	e := args.Error(1)
	return t.(uint64), e
}

func (n *VNode) Renew(ctx context.Context, lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	args := n.Called(ctx, lease, ttl, prevToken)
	t := args.Get(0)
	e := args.Error(1)
	return t.(uint64), e
}

func (n *VNode) Release(ctx context.Context, lease []byte, token uint64) error {
	args := n.Called(ctx, lease, token)
	e := args.Error(0)
	return e
}

func (n *VNode) Import(ctx context.Context, keys [][]byte, values []*protocol.KVTransfer) error {
	args := n.Called(ctx, keys, values)
	e := args.Error(0)
	return e
}

func (n *VNode) ID() uint64 {
	args := n.Called()
	v := args.Get(0)
	return v.(uint64)
}

func (n *VNode) Identity() *protocol.Node {
	args := n.Called()
	v := args.Get(0)
	if v == nil {
		return nil
	}
	return v.(*protocol.Node)
}

func (n *VNode) Ping() error {
	args := n.Called()
	e := args.Error(0)
	return e
}

func (n *VNode) Notify(predecessor chord.VNode) error {
	args := n.Called(predecessor)
	return args.Error(0)
}

func (n *VNode) FindSuccessor(key uint64) (chord.VNode, error) {
	args := n.Called(key)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(chord.VNode), e
}

func (n *VNode) GetSuccessors() ([]chord.VNode, error) {
	args := n.Called()
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.([]chord.VNode), e
}

func (n *VNode) GetPredecessor() (chord.VNode, error) {
	args := n.Called()
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(chord.VNode), e
}

func (n *VNode) RequestToJoin(joiner chord.VNode) (chord.VNode, []chord.VNode, error) {
	args := n.Called(joiner)
	p := args.Get(0)
	s := args.Get(1)
	e := args.Error(2)
	if e != nil {
		return nil, nil, e
	}
	return p.(chord.VNode), s.([]chord.VNode), e
}

func (n *VNode) FinishJoin(stablize bool, release bool) error {
	args := n.Called()
	e := args.Error(0)
	return e
}

func (n *VNode) RequestToLeave(leaver chord.VNode) error {
	args := n.Called(leaver)
	e := args.Error(0)
	return e
}

func (n *VNode) FinishLeave(stablize bool, release bool) error {
	args := n.Called()
	e := args.Error(0)
	return e
}
