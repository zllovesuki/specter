// Copyright (c) 2017, Tom Thorogood
// Copyright (c) 2021, Carlo Alberto Ferraris
// All rights reserved.
// Use of this source code is governed by a
// Modified BSD License that can be found in
// the LICENSE file.

// +build amd64,!gccgo,!appengine

#include "textflag.h"

TEXT ·swapUint128amd64(SB),NOSPLIT,$0
	MOVQ addr+0(FP), BP
    MOVQ 0(BP), AX
    MOVQ 8(BP), DX
	MOVQ new+8(FP), BX
	MOVQ new+16(FP), CX
loop:
	LOCK
	CMPXCHG16B (BP)
    JE done
    PAUSE
	JMP loop
done:
	MOVQ AX, old+24(FP)
	MOVQ DX, old+32(FP)
	RET

TEXT ·compareAndSwapUint128amd64(SB),NOSPLIT,$0
	MOVQ addr+0(FP), BP
	MOVQ old+8(FP), AX
	MOVQ old+16(FP), DX
	MOVQ new+24(FP), BX
	MOVQ new+32(FP), CX
	LOCK
	CMPXCHG16B (BP)
	SETEQ swapped+40(FP)
	RET

TEXT ·loadUint128amd64(SB),NOSPLIT,$0
	MOVQ addr+0(FP), BP
	XORQ AX, AX
	XORQ DX, DX
	XORQ BX, BX
	XORQ CX, CX
	LOCK
	CMPXCHG16B (BP)
	MOVQ AX, val+8(FP)
	MOVQ DX, val+16(FP)
	RET

TEXT ·storeUint128amd64(SB),NOSPLIT,$0
	MOVQ addr+0(FP), BP
    MOVQ 0(BP), AX
    MOVQ 8(BP), DX
	MOVQ new+8(FP), BX
	MOVQ new+16(FP), CX
loop:
	LOCK
	CMPXCHG16B (BP)
    JE done
    PAUSE
	JMP loop
done:
	RET

TEXT ·addUint128amd64(SB),NOSPLIT,$0
	MOVQ addr+0(FP), BP
    MOVQ 0(BP), AX
    MOVQ 8(BP), DX
    MOVQ incr+8(FP), SI
    MOVQ incr+16(FP), DI
loop:
    MOVQ AX, BX
    MOVQ DX, CX
    ADDQ SI, BX
    ADCQ DI, CX
	LOCK
	CMPXCHG16B (BP)
    JE done
    PAUSE
	JMP loop
done:
    MOVQ BX, val+24(FP)
    MOVQ CX, val+32(FP)
	RET    

TEXT ·andUint128amd64(SB),NOSPLIT,$0
	MOVQ addr+0(FP), BP
    MOVQ 0(BP), AX
    MOVQ 8(BP), DX
    MOVQ incr+8(FP), SI
    MOVQ incr+16(FP), DI
loop:
    MOVQ AX, BX
    MOVQ DX, CX
    ANDQ SI, BX
    ANDQ DI, CX
	LOCK
	CMPXCHG16B (BP)
    JE done
    PAUSE
	JMP loop
done:
    MOVQ BX, val+24(FP)
    MOVQ CX, val+32(FP)
	RET    

TEXT ·orUint128amd64(SB),NOSPLIT,$0
	MOVQ addr+0(FP), BP
    MOVQ 0(BP), AX
    MOVQ 8(BP), DX
    MOVQ incr+8(FP), SI
    MOVQ incr+16(FP), DI
loop:
    MOVQ AX, BX
    MOVQ DX, CX
    ORQ SI, BX
    ORQ DI, CX
	LOCK
	CMPXCHG16B (BP)
    JE done
    PAUSE
	JMP loop
done:
    MOVQ BX, val+24(FP)
    MOVQ CX, val+32(FP)
	RET    

TEXT ·xorUint128amd64(SB),NOSPLIT,$0
	MOVQ addr+0(FP), BP
    MOVQ 0(BP), AX
    MOVQ 8(BP), DX
    MOVQ incr+8(FP), SI
    MOVQ incr+16(FP), DI
loop:
    MOVQ AX, BX
    MOVQ DX, CX
    XORQ SI, BX
    XORQ DI, CX
	LOCK
	CMPXCHG16B (BP)
    JE done
    PAUSE
	JMP loop
done:
    MOVQ BX, val+24(FP)
    MOVQ CX, val+32(FP)
	RET    
