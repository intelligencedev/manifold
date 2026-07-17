// Command gemm generates the vendored AVX2+FMA GEMM micro-kernels for the CPU
// backend. It is run via `go generate` (see matmul_gemm_amd64.go) and lives in a
// separate module (_gen/gemm/go.mod) so avo never enters born's module graph.
// The generated artifacts (gemm_microkernel_amd64.s and its Go stub) are
// committed.
//
// Both kernels operate on PACKED panels: the driver copies A and B into
// contiguous tile-local buffers so the kernels stream them sequentially (the
// dominant front-end GEMM otherwise reads B with a column stride, thrashing the
// cache). C is held in registers across the entire k-loop and stored once.
//
//   - gemmMicroKernel6x16AVX2: a 6x16 register tile. A is packed [k][6] and B is
//     packed [k][16]; 12 YMM accumulators hold the tile across k.
//   - gemmMicroKernel1x16AVX2: the 1-row remainder / GEMV path. A is a single
//     unpacked source row [k]; B is the same packed [k][16] panel.
package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	"github.com/mmcloughlin/avo/reg"
)

const (
	mr = 6  // micro-kernel rows
	nr = 16 // micro-kernel columns (two YMM lanes of 8 float32)
)

func main() {
	// Match the dispatch file (matmul_gemm_amd64.go): the vendored kernel is the
	// default amd64 path, and a GOEXPERIMENT=simd build hands SIMD to the archsimd
	// micro-kernel instead, so exclude it there to keep the two paths mutually
	// exclusive. The .s and its Go stub share this constraint as a matched pair.
	ConstraintExpr("amd64,!goexperiment.simd")
	emitKernel6x16()
	emitKernel1x16()
	emitKernel1x16Strided()
	Generate()
}

// emitKernel6x16 generates the 6x16 packed micro-kernel.
//
//	C[r, col] = sum_kk aPacked[kk*6 + r] * bPacked[kk*16 + col]
//
// for r in 0..5, col in 0..15. aPacked and bPacked are tile-local contiguous
// panels; cStride is the C row stride (n of the full output, in elements).
func emitKernel6x16() {
	TEXT("gemmMicroKernel6x16AVX2", NOSPLIT, "func(c, a, b []float32, k, cStride int)")
	Doc("gemmMicroKernel6x16AVX2 computes a 6x16 tile C[r,col] = sum_kk a[kk*6+r]*b[kk*16+col]",
		"(overwriting C) from PACKED panels a ([k][6]) and b ([k][16]), holding all 12 YMM",
		"accumulators in registers across the k-loop. cStride is the C row stride in elements.")
	// The kernel reads/writes its slice arguments but never retains them, so the
	// pointers do not escape; this lets callers keep argument slices on the stack.
	Pragma("noescape")

	cBase := Load(Param("c").Base(), GP64())
	aPtr := Load(Param("a").Base(), GP64())
	bPtr := Load(Param("b").Base(), GP64())
	k := Load(Param("k"), GP64())
	cStride := Load(Param("cStride"), GP64())

	// C row stride in bytes (B and A advance by fixed packed strides, below).
	cStrideBytes := GP64()
	MOVQ(cStride, cStrideBytes)
	SHLQ(Imm(2), cStrideBytes)

	// Accumulators acc[r][v]: r in 0..5 rows, v in 0..1 (cols 0-7, 8-15). Zeroed.
	acc := [mr][2]reg.VecVirtual{}
	for r := 0; r < mr; r++ {
		for v := 0; v < 2; v++ {
			acc[r][v] = YMM()
			VXORPS(acc[r][v], acc[r][v], acc[r][v])
		}
	}

	kCtr := GP64()
	MOVQ(k, kCtr)

	Label("kloop6")
	CMPQ(kCtr, Imm(0))
	JE(LabelRef("kdone6"))

	bVec0 := YMM()
	bVec1 := YMM()
	VMOVUPS(Mem{Base: bPtr}, bVec0)
	VMOVUPS(Mem{Base: bPtr, Disp: 32}, bVec1)

	aVec := YMM()
	for r := 0; r < mr; r++ {
		VBROADCASTSS(Mem{Base: aPtr, Disp: r * 4}, aVec)
		VFMADD231PS(bVec0, aVec, acc[r][0]) // acc += aVec*bVec0
		VFMADD231PS(bVec1, aVec, acc[r][1])
	}

	ADDQ(Imm(mr*4), aPtr) // advance packed A by one row of mr float32
	ADDQ(Imm(nr*4), bPtr) // advance packed B by one row of nr float32
	DECQ(kCtr)
	JMP(LabelRef("kloop6"))

	Label("kdone6")
	// Store: running C-row pointer, advanced by cStrideBytes each row.
	cPtr := GP64()
	MOVQ(cBase, cPtr)
	for r := 0; r < mr; r++ {
		VMOVUPS(acc[r][0], Mem{Base: cPtr})
		VMOVUPS(acc[r][1], Mem{Base: cPtr, Disp: 32})
		if r != mr-1 {
			ADDQ(cStrideBytes, cPtr)
		}
	}

	// Clear the upper 128 bits of the YMM registers before returning to Go.
	// Without this, the dirty upper state forces the AVX->SSE transition penalty
	// on the surrounding (SSE-based) Go code. avo does not emit it automatically.
	VZEROUPPER()
	RET()
}

// emitKernel1x16 generates the 1-row x 16-col micro-kernel used for the 1-5
// remainder rows and for GEMV-shaped (m < mr) matmuls, so thin shapes stay
// vectorized over n instead of falling to a naive scalar loop. A is a single
// UNPACKED source row [k] (stride 4); B is the packed [k][16] panel (stride 64).
func emitKernel1x16() {
	TEXT("gemmMicroKernel1x16AVX2", NOSPLIT, "func(c, a, b []float32, k int)")
	Doc("gemmMicroKernel1x16AVX2 computes a 1x16 tile C[0,col] = sum_kk a[kk]*b[kk*16+col]",
		"(overwriting C) from an unpacked source row a ([k]) and the packed B panel b ([k][16]),",
		"with the two C accumulators held in registers across the k-loop.")
	Pragma("noescape")

	cBase := Load(Param("c").Base(), GP64())
	aPtr := Load(Param("a").Base(), GP64())
	bPtr := Load(Param("b").Base(), GP64())
	k := Load(Param("k"), GP64())

	acc0 := YMM()
	acc1 := YMM()
	VXORPS(acc0, acc0, acc0)
	VXORPS(acc1, acc1, acc1)

	kCtr := GP64()
	MOVQ(k, kCtr)

	Label("kloop1")
	CMPQ(kCtr, Imm(0))
	JE(LabelRef("kdone1"))

	aVec := YMM()
	VBROADCASTSS(Mem{Base: aPtr}, aVec)
	bVec0 := YMM()
	bVec1 := YMM()
	VMOVUPS(Mem{Base: bPtr}, bVec0)
	VMOVUPS(Mem{Base: bPtr, Disp: 32}, bVec1)
	VFMADD231PS(bVec0, aVec, acc0) // acc = a[kk]*bVec + acc
	VFMADD231PS(bVec1, aVec, acc1)

	ADDQ(Imm(4), aPtr)    // unpacked source row: one float32
	ADDQ(Imm(nr*4), bPtr) // packed B: one row of nr float32
	DECQ(kCtr)
	JMP(LabelRef("kloop1"))

	Label("kdone1")
	VMOVUPS(acc0, Mem{Base: cBase})
	VMOVUPS(acc1, Mem{Base: cBase, Disp: 32})
	VZEROUPPER()
	RET()
}

// emitKernel1x16Strided generates a 1x16 micro-kernel that reads B with its
// native row stride n (UNPACKED). The driver uses it for pure GEMV / very thin
// (m < mr) shapes, where packing B would double its memory traffic for no reuse
// (each B element is touched by only one output row). Both a and b are source
// slices: a is a single row [k] (stride 4), b is B[k,n] (row stride n).
func emitKernel1x16Strided() {
	TEXT("gemmMicroKernel1x16StridedAVX2", NOSPLIT, "func(c, a, b []float32, k, n int)")
	Doc("gemmMicroKernel1x16StridedAVX2 computes a 1x16 tile C[0,col] = sum_kk a[kk]*b[kk*n+col]",
		"(overwriting C) reading B with its native row stride n (unpacked). Used for GEMV / thin",
		"shapes where packing B would not pay off. c and a are tile-local; n is the B row stride.")
	Pragma("noescape")

	cBase := Load(Param("c").Base(), GP64())
	aPtr := Load(Param("a").Base(), GP64())
	bPtr := Load(Param("b").Base(), GP64())
	k := Load(Param("k"), GP64())
	n := Load(Param("n"), GP64())

	nBytes := GP64()
	MOVQ(n, nBytes)
	SHLQ(Imm(2), nBytes)

	acc0 := YMM()
	acc1 := YMM()
	VXORPS(acc0, acc0, acc0)
	VXORPS(acc1, acc1, acc1)

	kCtr := GP64()
	MOVQ(k, kCtr)

	Label("kloop1s")
	CMPQ(kCtr, Imm(0))
	JE(LabelRef("kdone1s"))

	aVec := YMM()
	VBROADCASTSS(Mem{Base: aPtr}, aVec)
	bVec0 := YMM()
	bVec1 := YMM()
	VMOVUPS(Mem{Base: bPtr}, bVec0)
	VMOVUPS(Mem{Base: bPtr, Disp: 32}, bVec1)
	VFMADD231PS(bVec0, aVec, acc0)
	VFMADD231PS(bVec1, aVec, acc1)

	ADDQ(Imm(4), aPtr)
	ADDQ(nBytes, bPtr) // unpacked B: advance by one source row
	DECQ(kCtr)
	JMP(LabelRef("kloop1s"))

	Label("kdone1s")
	VMOVUPS(acc0, Mem{Base: cBase})
	VMOVUPS(acc1, Mem{Base: cBase, Disp: 32})
	VZEROUPPER()
	RET()
}
