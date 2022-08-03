package mocks

import (
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

func (n *VNode) Import(keys [][]byte, values [][]byte) error {
	args := n.Called(keys, values)
	e := args.Error(0)
	return e
}

func (n *VNode) Export(keys [][]byte) [][]byte {
	args := n.Called(keys)
	v := args.Get(0)
	if v == nil {
		return nil
	}
	return v.([][]byte)
}

func (n *VNode) RangeKeys(low uint64, high uint64) [][]byte {
	args := n.Called(low, high)
	v := args.Get(0)
	if v == nil {
		return nil
	}
	return v.([][]byte)
}

func (n *VNode) RemoveKeys(keys [][]byte) {
	n.Called(keys)
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

func (n *VNode) Stop() {
	n.Called()
}
