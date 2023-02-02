// Package wyhash implements the wyhash hash algorithm as defined in github.com/wangyi-fudan/wyhash
package wyhash

import (
	"hash"
	"math/bits"
)

const (
	wyp0 uint64 = 0xa0761d6478bd642f
	wyp1 uint64 = 0xe7037ed1a0b428db
	wyp2 uint64 = 0x8ebc6af09c88c6e3
	wyp3 uint64 = 0x589965cc75374cc3
	wyp4 uint64 = 0x1d8e4e27c47d124f
	wyp5 uint64 = 0xeb44accab455d165
)

// The blocksize of wyhash in bytes.
const BlockSize = 32
// The size of a wyhash checksum in bytes.
const Size = 8

// New returns a new hash.Hash64 computing the wyhash checksum.
func New(seed uint64) hash.Hash64 {
	d := &digest{seed: seed}
	d.Reset()
	return d
}

// Sum64 returns the wyhash checksum of the b
func Sum64(seed uint64, b []byte) uint64 {
	h := New(seed)
	_, _ = h.Write(b)
	return h.Sum64()
}

func mum(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	return hi ^ lo
}

type digest struct {
	seed  uint64
	state uint64
	size  int
	buf   []byte
}

func read64(b []byte) uint64 {
	return read32(b)<<32 | read32(b[4:])
}

func read32(b []byte) uint64 {
	_ = b[3]
	x := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	return uint64(x)
}

func read16(b []byte) uint64 {
	_ = b[1]
	x := uint16(b[0]) | uint16(b[1])<<8
	return uint64(x)
}

func read8(b []byte) uint64 {
	x := uint8(b[0])
	return uint64(x)
}

func (d *digest) Write(p []byte) (int, error) {
	seed := d.state
	n := len(p)
	d.size += n

	buffered := len(d.buf)
	if buffered > 0 {
		rest := BlockSize - buffered
		if len(p) < rest {
			d.buf = append(d.buf, p...)
			return n, nil
		}
		d.buf = append(d.buf, p[:rest]...)
		seed = consumeBlock(seed, d.buf)
		d.buf = d.buf[:0]
		p = p[rest:]
	}
	for i, size := 0, len(p); i+BlockSize <= size; i += BlockSize {
		seed = consumeBlock(seed, p)
		p = p[BlockSize:]
	}
	if len(p) != 0 {
		d.buf = append(d.buf, p...)
	}
	d.state = seed
	return n, nil
}

func (d *digest) Sum(b []byte) []byte {
	x := d.Sum64()
	return append(b, byte((x>>56)&0xff), byte((x>>48)&0xff), byte((x>>40)&0xff), byte((x>>32)&0xff), byte((x>>24)&0xff), byte((x>>16)&0xff), byte((x>>8)&0xff), byte(x&0xff))
}

func (d *digest) Sum64() uint64 {
	seed := d.state
	seed ^= wyp0
	p := d.buf
	switch d.size & (BlockSize - 1) {
	case 1:
		seed = mum(seed, read8(p)^wyp1)
	case 2:
		seed = mum(seed, read16(p)^wyp1)
	case 3:
		seed = mum(seed, (read16(p)<<8|read8(p[2:]))^wyp1)
	case 4:
		seed = mum(seed, read32(p)^wyp1)
	case 5:
		seed = mum(seed, (read32(p)<<8|read8(p[4:]))^wyp1)
	case 6:
		seed = mum(seed, (read32(p)<<16|read16(p[4:]))^wyp1)
	case 7:
		seed = mum(seed, (read32(p)<<24|read16(p[4:])<<8|read8(p[6:]))^wyp1)
	case 8:
		seed = mum(seed, read64(p)^wyp1)
	case 9:
		seed = mum(read64(p)^seed, read8(p[8:])^wyp2)
	case 10:
		seed = mum(read64(p)^seed, read16(p[8:])^wyp2)
	case 11:
		seed = mum(read64(p)^seed, ((read16(p[8:])<<8)|read8(p[10:]))^wyp2)
	case 12:
		seed = mum(read64(p)^seed, read32(p[8:])^wyp2)
	case 13:
		seed = mum(read64(p)^seed, ((read32(p[8:])<<8)|read8(p[12:]))^wyp2)
	case 14:
		seed = mum(read64(p)^seed, ((read32(p[8:])<<16)|read16(p[12:]))^wyp2)
	case 15:
		seed = mum(read64(p)^seed, ((read32(p[8:])<<24)|(read16(p[12:])<<8)|read8(p[14:]))^wyp2)
	case 16:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2)
	case 17:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(seed, read8(p[16:])^wyp3)
	case 18:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(seed, read16(p[16:])^wyp3)
	case 19:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(seed, (read16(p[16:])<<8)|read8(p[18:])^wyp3)
	case 20:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(seed, read32(p[16:])^wyp3)
	case 21:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(seed, (read32(p[16:])<<8)|read8(p[20:])^wyp3)
	case 22:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(seed, (read32(p[16:])<<16)|read16(p[20:])^wyp3)
	case 23:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(seed, (read32(p[16:])<<24)|(read16(p[20:])<<8)|read8(p[22:])^wyp3)
	case 24:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(seed, read64(p[16:])^wyp3)
	case 25:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(read64(p[16:])^seed, read8(p[24:])^wyp4)
	case 26:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(read64(p[16:])^seed, read16(p[24:])^wyp4)
	case 27:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(read64(p[16:])^seed, ((read16(p[24:])<<8)|read8(p[26:]))^wyp4)
	case 28:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(read64(p[16:])^seed, read32(p[24:])^wyp4)
	case 29:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(read64(p[16:])^seed, ((read32(p[24:])<<8)|read8(p[28:]))^wyp4)
	case 30:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(read64(p[16:])^seed, ((read32(p[24:])<<16)|read16(p[28:]))^wyp4)
	case 31:
		seed = mum(read64(p)^seed, read64(p[8:])^wyp2) ^ mum(read64(p[16:])^seed, ((read32(p[24:])<<24)|(read16(p[28:])<<8)|read8(p[30:]))^wyp4)
	}
	return mum(seed, uint64(d.size)^wyp5)
}

func (d *digest) Reset() {
	d.state = d.seed
	d.size = 0
	if d.buf != nil {
		d.buf = d.buf[:0]
	}
}

func (d *digest) Size() int {
	return Size
}

func (d *digest) BlockSize() int {
	return BlockSize
}

func consumeBlock(seed uint64, b []byte) uint64 {
	_ = b[31]
	p1 := uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
	p2 := uint64(b[8]) | uint64(b[9])<<8 | uint64(b[10])<<16 | uint64(b[11])<<24 |
		uint64(b[12])<<32 | uint64(b[13])<<40 | uint64(b[14])<<48 | uint64(b[15])<<56
	p3 := uint64(b[16]) | uint64(b[17])<<8 | uint64(b[18])<<16 | uint64(b[19])<<24 |
		uint64(b[20])<<32 | uint64(b[21])<<40 | uint64(b[22])<<48 | uint64(b[23])<<56
	p4 := uint64(b[24]) | uint64(b[25])<<8 | uint64(b[26])<<16 | uint64(b[27])<<24 |
		uint64(b[28])<<32 | uint64(b[29])<<40 | uint64(b[30])<<48 | uint64(b[31])<<56
	return mum(seed^wyp0, mum(p1^wyp1, p2^wyp2)^mum(p3^wyp3, p4^wyp4))
}
