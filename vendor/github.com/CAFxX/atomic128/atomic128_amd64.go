// +build amd64,!gccgo,!appengine

package atomic128

import "github.com/klauspost/cpuid/v2"

func compareAndSwapUint128amd64(*[2]uint64, [2]uint64, [2]uint64) bool
func loadUint128amd64(*[2]uint64) [2]uint64
func storeUint128amd64(*[2]uint64, [2]uint64)
func swapUint128amd64(*[2]uint64, [2]uint64) [2]uint64
func addUint128amd64(ptr *[2]uint64, incr [2]uint64) [2]uint64
func andUint128amd64(ptr *[2]uint64, incr [2]uint64) [2]uint64
func orUint128amd64(ptr *[2]uint64, incr [2]uint64) [2]uint64
func xorUint128amd64(ptr *[2]uint64, incr [2]uint64) [2]uint64

func init() {
	if !cpuid.CPU.Supports(cpuid.CX16) {
		return
	}
	compareAndSwapUint128 = compareAndSwapUint128amd64
	loadUint128 = loadUint128amd64
	storeUint128 = storeUint128amd64
	swapUint128 = swapUint128amd64
	addUint128 = addUint128amd64
	andUint128 = andUint128amd64
	orUint128 = orUint128amd64
	xorUint128 = xorUint128amd64
}
