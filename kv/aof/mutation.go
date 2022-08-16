package aof

import (
	"io/fs"

	"kon.nect.sh/specter/kv/aof/proto"
	"kon.nect.sh/specter/spec/protocol"

	"go.uber.org/zap"
)

func (d *DiskKV) handleMutation(mut *proto.Mutation) error {
	var err error

	d.logger.Debug("Handling mutation", zap.String("mutation", mut.GetType().String()))

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

func (d *DiskKV) Put(key []byte, value []byte) error {
	d.writeBarrier.RLock()
	defer d.writeBarrier.RUnlock()
	if d.closed.Load() {
		return fs.ErrClosed
	}

	req := mutationReq{
		err: make(chan error),
		mut: &proto.Mutation{
			Type:  proto.MutationType_SIMPLE_PUT,
			Key:   key,
			Value: value,
		},
	}
	d.queue <- req
	return <-req.err
}

func (d *DiskKV) Delete(key []byte) error {
	d.writeBarrier.RLock()
	defer d.writeBarrier.RUnlock()
	if d.closed.Load() {
		return fs.ErrClosed
	}

	req := mutationReq{
		err: make(chan error),
		mut: &proto.Mutation{
			Type: proto.MutationType_SIMPLE_DELETE,
			Key:  key,
		},
	}
	d.queue <- req
	return <-req.err
}

func (d *DiskKV) PrefixAppend(prefix []byte, child []byte) error {
	d.writeBarrier.RLock()
	defer d.writeBarrier.RUnlock()
	if d.closed.Load() {
		return fs.ErrClosed
	}

	req := mutationReq{
		err: make(chan error),
		mut: &proto.Mutation{
			Type:  proto.MutationType_PREFIX_APPEND,
			Key:   prefix,
			Value: child,
		},
	}
	d.queue <- req
	return <-req.err
}

func (d *DiskKV) PrefixRemove(prefix []byte, child []byte) error {
	d.writeBarrier.RLock()
	defer d.writeBarrier.RUnlock()
	if d.closed.Load() {
		return fs.ErrClosed
	}

	req := mutationReq{
		err: make(chan error),
		mut: &proto.Mutation{
			Type:  proto.MutationType_PREFIX_REMOVE,
			Key:   prefix,
			Value: child,
		},
	}
	d.queue <- req
	return <-req.err
}

func (d *DiskKV) Import(keys [][]byte, values []*protocol.KVTransfer) error {
	d.writeBarrier.RLock()
	defer d.writeBarrier.RUnlock()
	if d.closed.Load() {
		return fs.ErrClosed
	}

	req := mutationReq{
		err: make(chan error),
		mut: &proto.Mutation{
			Type:   proto.MutationType_IMPORT,
			Keys:   keys,
			Values: values,
		},
	}
	d.queue <- req
	return <-req.err
}

func (d *DiskKV) RemoveKeys(keys [][]byte) {
	d.writeBarrier.RLock()
	defer d.writeBarrier.RUnlock()
	if d.closed.Load() {
		return
	}

	req := mutationReq{
		err: make(chan error),
		mut: &proto.Mutation{
			Type: proto.MutationType_REMOVE_KEYS,
			Keys: keys,
		},
	}
	d.queue <- req
	<-req.err
}
