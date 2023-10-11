package aof

import (
	"context"

	"go.miragespace.co/specter/spec/protocol"
)

func (d *DiskKV) Get(ctx context.Context, key []byte) (value []byte, err error) {
	return d.memKv.Get(ctx, key)
}

func (d *DiskKV) PrefixList(ctx context.Context, prefix []byte) (children [][]byte, err error) {
	return d.memKv.PrefixList(ctx, prefix)
}

func (d *DiskKV) PrefixContains(ctx context.Context, prefix []byte, child []byte) (bool, error) {
	return d.memKv.PrefixContains(ctx, prefix, child)
}

func (d *DiskKV) ListKeys(ctx context.Context, prefix []byte) ([]*protocol.KeyComposite, error) {
	return d.memKv.ListKeys(ctx, prefix)
}

func (d *DiskKV) Export(keys [][]byte) []*protocol.KVTransfer {
	return d.memKv.Export(keys)
}

func (d *DiskKV) RangeKeys(low, high uint64) [][]byte {
	return d.memKv.RangeKeys(low, high)
}
