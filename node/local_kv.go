package node

import "specter/chord"

func (n *LocalNode) Put(key, value []byte) error {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return err
	}
	// n.conf.Logger.Debug("KV Put", zap.Binary("key", key), zap.Uint64("id", id), zap.Uint64("node", succ.ID()))
	if succ.ID() == n.ID() {
		return n.kv.Put(key, value)
	}
	return succ.Put(key, value)
}

func (n *LocalNode) Get(key []byte) ([]byte, error) {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return nil, err
	}
	if succ.ID() == n.ID() {
		return n.kv.Get(key)
	}
	return succ.Get(key)
}

func (n *LocalNode) Delete(key []byte) error {
	id := chord.Hash(key)
	succ, err := n.FindSuccessor(id)
	if err != nil {
		return err
	}
	if succ.ID() == n.ID() {
		return n.kv.Delete(key)
	}
	return succ.Delete(key)
}
