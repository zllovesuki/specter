package server

import (
	"github.com/zllovesuki/specter/spec/chord"
	"github.com/zllovesuki/specter/spec/protocol"

	"github.com/stretchr/testify/mock"
)

type mockVNode struct {
	mock.Mock
}

var _ chord.VNode = (*mockVNode)(nil)

func (n *mockVNode) Put(key []byte, value []byte) error {
	args := n.Called(key, value)
	return args.Error(0)
}

func (n *mockVNode) Get(key []byte) (value []byte, err error) {
	args := n.Called(key)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.([]byte), e
}

func (n *mockVNode) Delete(key []byte) error {
	args := n.Called(key)
	return args.Error(0)
}

func (n *mockVNode) LocalKeys(low uint64, high uint64) ([][]byte, error) {
	args := n.Called(low, high)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.([][]byte), e
}

func (n *mockVNode) LocalPuts(keys [][]byte, values [][]byte) error {
	args := n.Called(keys, values)
	e := args.Error(0)
	return e
}

func (n *mockVNode) LocalGets(keys [][]byte) ([][]byte, error) {
	args := n.Called(keys)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.([][]byte), e
}

func (n *mockVNode) LocalDeletes(keys [][]byte) error {
	args := n.Called(keys)
	e := args.Error(0)
	return e
}

func (n *mockVNode) ID() uint64 {
	args := n.Called()
	v := args.Get(0)
	return v.(uint64)
}

func (n *mockVNode) Identity() *protocol.Node {
	args := n.Called()
	v := args.Get(0)
	if v == nil {
		return nil
	}
	return v.(*protocol.Node)
}

func (n *mockVNode) Ping() error {
	args := n.Called()
	e := args.Error(0)
	return e
}

func (n *mockVNode) Notify(predecessor chord.VNode) error {
	args := n.Called(predecessor)
	return args.Error(0)
}

func (n *mockVNode) FindSuccessor(key uint64) (chord.VNode, error) {
	args := n.Called(key)
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(chord.VNode), e
}

func (n *mockVNode) GetSuccessors() ([]chord.VNode, error) {
	args := n.Called()
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.([]chord.VNode), e
}

func (n *mockVNode) GetPredecessor() (chord.VNode, error) {
	args := n.Called()
	v := args.Get(0)
	e := args.Error(1)
	if v == nil {
		return nil, e
	}
	return v.(chord.VNode), e
}

func (n *mockVNode) Stop() {
	n.Called()
}
