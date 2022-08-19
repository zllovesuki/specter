package aof

import (
	"io/fs"
	"sync"

	"kon.nect.sh/specter/kv/aof/proto"
	"kon.nect.sh/specter/spec/protocol"
)

var reqPool = sync.Pool{
	New: func() any {
		return &mutationReq{
			err: make(chan error),
			mut: proto.MutationFromVTPool(),
		}
	},
}

func (d *DiskKV) handleMutation(mut *proto.Mutation) error {
	var err error

	switch mut.GetType() {
	case proto.MutationType_SIMPLE_PUT:
		err = d.memKv.Put(mut.GetKey(), mut.GetValue())

	case proto.MutationType_SIMPLE_DELETE:
		err = d.memKv.Delete(mut.GetKey())

	case proto.MutationType_PREFIX_APPEND:
		err = d.memKv.PrefixAppend(mut.GetKey(), mut.GetValue())

	case proto.MutationType_PREFIX_REMOVE:
		err = d.memKv.PrefixRemove(mut.GetKey(), mut.GetValue())

	case proto.MutationType_IMPORT:
		err = d.memKv.Import(mut.GetKeys(), mut.GetValues())

	case proto.MutationType_REMOVE_KEYS:
		d.memKv.RemoveKeys(mut.GetKeys())

	}
	return err
}

func (d *DiskKV) mutationHandler(fn func(mut *proto.Mutation)) error {
	d.writeBarrier.RLock()
	defer d.writeBarrier.RUnlock()
	if d.closed.Load() {
		return fs.ErrClosed
	}

	req := reqPool.Get().(*mutationReq)
	defer func() {
		req.mut.ResetVT()
		reqPool.Put(req)
	}()

	fn(req.mut)

	d.queue <- req
	return <-req.err
}

func (d *DiskKV) Put(key []byte, value []byte) error {
	return d.mutationHandler(func(mut *proto.Mutation) {
		mut.Type = proto.MutationType_SIMPLE_PUT
		mut.Key = key
		mut.Value = value
	})
}

func (d *DiskKV) Delete(key []byte) error {
	return d.mutationHandler(func(mut *proto.Mutation) {
		mut.Type = proto.MutationType_SIMPLE_DELETE
		mut.Key = key
	})
}

func (d *DiskKV) PrefixAppend(prefix []byte, child []byte) error {
	return d.mutationHandler(func(mut *proto.Mutation) {
		mut.Type = proto.MutationType_PREFIX_APPEND
		mut.Key = prefix
		mut.Value = child
	})
}

func (d *DiskKV) PrefixRemove(prefix []byte, child []byte) error {
	return d.mutationHandler(func(mut *proto.Mutation) {
		mut.Type = proto.MutationType_PREFIX_REMOVE
		mut.Key = prefix
		mut.Value = child
	})
}

func (d *DiskKV) Import(keys [][]byte, values []*protocol.KVTransfer) error {
	return d.mutationHandler(func(mut *proto.Mutation) {
		mut.Type = proto.MutationType_IMPORT
		mut.Keys = keys
		mut.Values = values
	})
}

func (d *DiskKV) RemoveKeys(keys [][]byte) {
	d.mutationHandler(func(mut *proto.Mutation) {
		mut.Type = proto.MutationType_REMOVE_KEYS
		mut.Keys = keys
	})
}
