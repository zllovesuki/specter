//go:build !no_mocks
// +build !no_mocks

package mocks

import (
	"time"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
)

type VNode struct {
	mock.Mock
}

var _ chord.VNode = (*VNode)(nil)

func (n *VNode) Put(key []byte, value []byte) error {
	args := n.Called(key, value)
	return args.Error(0)
}

func (n *VNode) Get(key []byte) (value []byte, err error) {
	args := n.Called(key)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.([]byte), e
}

func (n *VNode) Delete(key []byte) error {
	args := n.Called(key)
	return args.Error(0)
}

func (n *VNode) PrefixAppend(prefix []byte, child []byte) error {
	args := n.Called(prefix, child)
	e := args.Error(0)
	return e
}

func (n *VNode) PrefixList(prefix []byte) ([][]byte, error) {
	args := n.Called(prefix)
	v := args.Get(0)
	e := args.Error(1)
	return v.([][]byte), e
}

func (n *VNode) PrefixContains(prefix []byte, child []byte) (bool, error) {
	args := n.Called(prefix, child)
	v := args.Bool(0)
	e := args.Error(1)
	return v, e
}

func (n *VNode) PrefixRemove(prefix []byte, child []byte) error {
	args := n.Called(prefix, child)
	e := args.Error(0)
	return e
}

func (n *VNode) Acquire(lease []byte, ttl time.Duration) (token uint64, err error) {
	args := n.Called(lease, ttl)
	t := args.Get(0)
	e := args.Error(1)
	return t.(uint64), e
}

func (n *VNode) Renew(lease []byte, ttl time.Duration, prevToken uint64) (newToken uint64, err error) {
	args := n.Called(lease, ttl, prevToken)
	t := args.Get(0)
	e := args.Error(1)
	return t.(uint64), e
}

func (n *VNode) Release(lease []byte, token uint64) error {
	args := n.Called(lease, token)
	e := args.Error(0)
	return e
}

func (n *VNode) Import(keys [][]byte, values []*protocol.KVTransfer) error {
	args := n.Called(keys, values)
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
