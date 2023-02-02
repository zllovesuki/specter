// Package atomic128 implements atomic operations on 128 bit values.
// When possible (e.g. on amd64 processors that support CMPXCHG16B), it automatically uses
// native CPU features to implement the operations; otherwise it falls back to an approach
// based on mutexes.
package atomic128

import (
	"sync"
	"unsafe"
)

// Uint128 is an opaque container for an atomic uint128.
// Uint128 must not be copied.
// The zero value is a valid value representing [2]uint64{0, 0}.
type Uint128 struct {
	// d is protected by m in the fallback code path; it is placed first because
	// addr() relies on the 64-bit alignment guarantee: see
	// https://go101.org/article/memory-layout.html and,
	// specifically, https://pkg.go.dev/sync/atomic#pkg-note-BUG.
	d [3]uint64
	m sync.Mutex
}

// CompareAndSwapUint128 performs a 128-bit atomic CAS on ptr.
// If the memory pointed to by ptr contains the value old, it is set to
// the value new, and true is returned. Otherwise the memory pointed to
// by ptr is unchanged, and false is returned.
// In the old and new values the first of the two elements is the low-order bits.
func CompareAndSwapUint128(ptr *Uint128, old, new [2]uint64) bool {
	if compareAndSwapUint128 != nil {
		return compareAndSwapUint128(addr(ptr), old, new)
	}

	ptr.m.Lock()
	v := load(ptr)
	if v != old {
		ptr.m.Unlock()
		return false
	}
	store(ptr, new)
	ptr.m.Unlock()
	return true
}

// LoadUint128 atomically loads the 128 bit value pointed to by ptr.
// In the returned value the first of the two elements is the low-order bits.
func LoadUint128(ptr *Uint128) [2]uint64 {
	if loadUint128 != nil {
		return loadUint128(addr(ptr))
	}

	ptr.m.Lock()
	v := load(ptr)
	ptr.m.Unlock()
	return v
}

// StoreUint128 atomically stores the new value in the 128 bit value pointed to by ptr.
// In the new value the first of the two elements is the low-order bits.
func StoreUint128(ptr *Uint128, new [2]uint64) {
	if storeUint128 != nil {
		storeUint128(addr(ptr), new)
		return
	}

	ptr.m.Lock()
	store(ptr, new)
	ptr.m.Unlock()
}

// SwapUint128 atomically stores the new value with the 128 bit value pointed to by ptr,
// and it returns the 128 bit value that was previously pointed to by ptr.
// In the new and returned values the first of the two elements is the low-order bits.
func SwapUint128(ptr *Uint128, new [2]uint64) [2]uint64 {
	if swapUint128 != nil {
		return swapUint128(addr(ptr), new)
	}

	ptr.m.Lock()
	old := load(ptr)
	store(ptr, new)
	ptr.m.Unlock()
	return old
}

// AddUint128 atomically adds the incr value to the 128 bit value pointed to by ptr,
// and it returns the resulting 128 bit value.
// In the incr and returned values the first of the two elements is the low-order bits.
func AddUint128(ptr *Uint128, incr [2]uint64) [2]uint64 {
	if addUint128 != nil {
		return addUint128(addr(ptr), incr)
	}

	ptr.m.Lock()
	v := load(ptr)
	v[0] += incr[0]
	if v[0] < incr[0] {
		v[1]++
	}
	v[1] += incr[1]
	store(ptr, v)
	ptr.m.Unlock()
	return v
}

// AndUint128 atomically performs a bitwise AND of the op value to the 128 bit value pointed to by ptr,
// and it returns the resulting 128 bit value.
// In the op and returned values the first of the two elements is the low-order bits.
func AndUint128(ptr *Uint128, op [2]uint64) [2]uint64 {
	if andUint128 != nil {
		return andUint128(addr(ptr), op)
	}

	ptr.m.Lock()
	v := load(ptr)
	v[0] &= op[0]
	v[1] &= op[1]
	store(ptr, v)
	ptr.m.Unlock()
	return v
}

// OrUint128 atomically performs a bitwise OR of the op value to the 128 bit value pointed to by ptr,
// and it returns the resulting 128 bit value.
// In the op and returned values the first of the two elements is the low-order bits.
func OrUint128(ptr *Uint128, op [2]uint64) [2]uint64 {
	if orUint128 != nil {
		return orUint128(addr(ptr), op)
	}

	ptr.m.Lock()
	v := load(ptr)
	v[0] |= op[0]
	v[1] |= op[1]
	store(ptr, v)
	ptr.m.Unlock()
	return v
}

// XorUint128 atomically performs a bitwise XOR of the op value to the 128 bit value pointed to by ptr,
// and it returns the resulting 128 bit value.
// In the op and returned values the first of the two elements is the low-order bits.
func XorUint128(ptr *Uint128, op [2]uint64) [2]uint64 {
	if xorUint128 != nil {
		return xorUint128(addr(ptr), op)
	}

	ptr.m.Lock()
	v := load(ptr)
	v[0] ^= op[0]
	v[1] ^= op[1]
	store(ptr, v)
	ptr.m.Unlock()
	return v
}

func addr(ptr *Uint128) *[2]uint64 {
	if (uintptr)((unsafe.Pointer)(&ptr.d[0]))%16 == 0 {
		return (*[2]uint64)((unsafe.Pointer)(&ptr.d[0]))
	}
	return (*[2]uint64)((unsafe.Pointer)(&ptr.d[1]))
}

func load(ptr *Uint128) [2]uint64 {
	return [2]uint64{ptr.d[0], ptr.d[1]}
}

func store(ptr *Uint128, v [2]uint64) {
	ptr.d[0], ptr.d[1] = v[0], v[1]
}

var (
	compareAndSwapUint128 func(*[2]uint64, [2]uint64, [2]uint64) bool
	loadUint128           func(*[2]uint64) [2]uint64
	storeUint128          func(*[2]uint64, [2]uint64)
	swapUint128           func(*[2]uint64, [2]uint64) [2]uint64
	addUint128            func(*[2]uint64, [2]uint64) [2]uint64
	andUint128            func(*[2]uint64, [2]uint64) [2]uint64
	orUint128             func(*[2]uint64, [2]uint64) [2]uint64
	xorUint128            func(*[2]uint64, [2]uint64) [2]uint64
)
