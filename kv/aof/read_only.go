package aof

import "kon.nect.sh/specter/spec/protocol"

func (d *DiskKV) Get(key []byte) (value []byte, err error) {
	return d.memKv.Get(key)
}

func (d *DiskKV) PrefixList(prefix []byte) (children [][]byte, err error) {
	return d.memKv.PrefixList(prefix)
}

func (d *DiskKV) PrefixContains(prefix []byte, child []byte) (bool, error) {
	return d.memKv.PrefixContains(prefix, child)
}

func (d *DiskKV) Export(keys [][]byte) []*protocol.KVTransfer {
	return d.memKv.Export(keys)
}

func (d *DiskKV) RangeKeys(low, high uint64) [][]byte {
	return d.memKv.RangeKeys(low, high)
}
