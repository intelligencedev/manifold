//go:build amd64 && !goexperiment.simd

package cpu

//go:generate sh -c "cd _gen/gemm && go run . -out ../../gemm_microkernel_amd64.s -stubs ../../gemm_microkernel_stub_amd64.go -pkg cpu"

import (
	"sync"

	"golang.org/x/sys/cpu"
)

// mr, nr are the GEMM micro-kernel tile dimensions and MUST match the constants
// in _gen/gemm/main.go (the packed-panel strides are baked into the asm). The
// kernel holds a mr×nr tile of C in registers across the k-loop.
const (
	gemmMr = 6
	gemmNr = 16
)

// gemmScratch holds the per-call packing buffers. They are pooled and grown in
// place so the GEMM fast path stays allocation-free across calls (one inference
// reuses the same backing arrays for every large multiply).
type gemmScratch struct {
	ap []float32 // packed A:        [nBlocks][k][gemmMr]
	bp []float32 // packed B:        [nTiles][k][gemmNr]
	bt []float32 // packed tail B:   [k][gemmNr] (n%gemmNr cols, zero-padded)
}

var gemmScratchPool = sync.Pool{New: func() any { return new(gemmScratch) }}

// ensureCap returns *buf resliced to length n, growing the backing array only
// when the current capacity is insufficient.
func ensureCap(buf *[]float32, n int) []float32 {
	if cap(*buf) < n {
		*buf = make([]float32, n)
	}
	return (*buf)[:n]
}

// init wires the vendored AVX2+FMA GEMM kernel into the matmul dispatch whenever
// the CPU supports AVX2+FMA. The kernel compiles into every default amd64 build
// (no build tag or env flag needed), and dispatch is decided here at startup from
// runtime CPU detection; CPUs without AVX2+FMA leave gemmF32 nil and use scalar.
func init() {
	if cpu.X86.HasAVX2 && cpu.X86.HasFMA {
		gemmF32 = gemmAVX2F32
	}
}

// gemmAVX2F32 computes C[m,n] = A[m,k] @ B[k,n] (row-major, overwriting C).
//
// A and B are packed into contiguous tile-local panels so the micro-kernels
// stream them sequentially: B is the dominant operand (the front-end GEMM has
// k=2048, n=1025) and reading it with a column stride otherwise thrashes the
// cache. The nr-wide column tiles are processed outermost so each packed B
// panel stays resident across all the row blocks that reuse it. Full gemmMr×gemmNr
// tiles run the 6x16 kernel (C held in registers across k); the 1-5 row tail and
// GEMV (m < gemmMr) shapes use the 1x16 kernel; the n%gemmNr column tail is run
// through the 1x16 kernel over a zero-padded packed panel.
func gemmAVX2F32(c, a, b []float32, m, k, n int) {
	if k == 0 {
		// Empty inner dimension: the product is the zero matrix. The matmulFloat32
		// dispatch already excludes k==0 (m*k*n < blockThreshold), but guard here so
		// a direct call cannot reslice b past its zero length in the tile loop.
		clear(c[:m*n])
		return
	}

	mFull := m - m%gemmMr
	nFull := n - n%gemmNr
	nrem := n - nFull

	sc := gemmScratchPool.Get().(*gemmScratch)
	defer gemmScratchPool.Put(sc)

	switch {
	case nFull == 0:
		// Every column is below one full tile; the tail path below covers them all.
	case mFull == 0:
		gemvStridedF32(c, a, b, m, k, n, nFull) // thin / GEMV: no packing pays off
	default:
		gemmPackedF32(c, a, b, m, k, n, mFull, nFull, sc)
	}

	if nrem > 0 {
		gemmTailF32(c, a, b, m, k, n, nFull, nrem, sc)
	}
}

// gemvStridedF32 handles thin shapes (m < gemmMr) over the full nr-wide column
// tiles. Each B element feeds only one output row, so packing B would double its
// traffic for no reuse; the 1x16 kernel streams B with its native stride.
func gemvStridedF32(c, a, b []float32, m, k, n, nFull int) {
	for i := 0; i < m; i++ {
		for j := 0; j < nFull; j += gemmNr {
			gemmMicroKernel1x16StridedAVX2(c[i*n+j:], a[i*k:], b[j:], k, n)
		}
	}
}

// gemmPackedF32 runs the packed 6x16 path for the full gemmMr×gemmNr tile region
// (m >= gemmMr, n >= gemmNr). A and B are packed once into pooled scratch; column
// tiles are processed outermost so each packed B panel stays L2-resident across
// the row blocks that reuse it.
func gemmPackedF32(c, a, b []float32, m, k, n, mFull, nFull int, sc *gemmScratch) {
	nTiles := nFull / gemmNr
	nBlocks := mFull / gemmMr

	bp := ensureCap(&sc.bp, nTiles*k*gemmNr)
	ap := ensureCap(&sc.ap, nBlocks*k*gemmMr)

	packB16(bp, b, k, n, nTiles)
	packA6(ap, a, k, nBlocks)

	for t := 0; t < nTiles; t++ {
		jt := t * gemmNr
		bpt := bp[t*k*gemmNr:]
		for bi := 0; bi < nBlocks; bi++ {
			gemmMicroKernel6x16AVX2(c[bi*gemmMr*n+jt:], ap[bi*k*gemmMr:], bpt, k, n)
		}
		// Remainder rows [mFull, m): one per call, reusing the already-packed panel
		// (cost-free since B is packed for the full blocks anyway).
		for i := mFull; i < m; i++ {
			gemmMicroKernel1x16AVX2(c[i*n+jt:], a[i*k:], bpt, k)
		}
	}
}

// gemmTailF32 computes the n%gemmNr column remainder [nFull, n) for all rows. The
// nrem (< gemmNr) tail columns of B are packed into one zero-padded [k][gemmNr]
// panel and run through the 1x16 kernel a row at a time: the kernel produces a
// full 16-wide result in SIMD lanes (the padded columns cost nothing) and only the
// nrem valid columns are stored. This replaces a scalar dot product that was a top
// hotspot for shapes like n=24 (nrem=8) and n=1025/513 (nrem=1).
func gemmTailF32(c, a, b []float32, m, k, n, nFull, nrem int, sc *gemmScratch) {
	bt := ensureCap(&sc.bt, k*gemmNr)
	packTailB(bt, b, k, n, nFull, nrem)

	var scratch [gemmNr]float32
	for i := 0; i < m; i++ {
		gemmMicroKernel1x16AVX2(scratch[:], a[i*k:], bt, k)
		copy(c[i*n+nFull:i*n+n], scratch[:nrem])
	}
}

// packTailB packs the nrem (< gemmNr) tail columns [nFull, n) of B[k,n] into bt as
// a contiguous [k][gemmNr] panel, zero-filling the unused columns so the 1x16
// kernel reads a full 16-wide row.
func packTailB(bt, b []float32, k, n, nFull, nrem int) {
	for kk := 0; kk < k; kk++ {
		d := bt[kk*gemmNr : kk*gemmNr+gemmNr : kk*gemmNr+gemmNr]
		copy(d[:nrem], b[kk*n+nFull:kk*n+nFull+nrem])
		for j := nrem; j < gemmNr; j++ {
			d[j] = 0
		}
	}
}

// packB16 copies the full gemmNr-wide column tiles of B[k,n] into bp laid out as
// [nTiles][k][gemmNr] contiguous, so the micro-kernel reads each panel's k rows
// sequentially (stride gemmNr) instead of with B's column stride n.
func packB16(bp, b []float32, k, n, nTiles int) {
	for t := 0; t < nTiles; t++ {
		jt := t * gemmNr
		dst := bp[t*k*gemmNr:]
		for kk := 0; kk < k; kk++ {
			copy(dst[kk*gemmNr:kk*gemmNr+gemmNr], b[kk*n+jt:kk*n+jt+gemmNr])
		}
	}
}

// packA6 copies the full gemmMr-tall row blocks of A[m,k] into ap laid out as
// [nBlocks][k][gemmMr] contiguous (transposed within a block), so the 6x16
// kernel reads the gemmMr A values for a given k as one contiguous group.
//
// Specialized to gemmMr == 6 (matching the 6x16 asm kernel): the six length-k
// source rows are sliced up front and the destination window is 3-index sliced,
// so the inner loop carries a single bounds check per k instead of twelve.
func packA6(ap, a []float32, k, nBlocks int) {
	for bi := 0; bi < nBlocks; bi++ {
		base := bi * gemmMr * k
		dst := ap[base : base+gemmMr*k]
		r0 := a[base+0*k : base+1*k]
		r1 := a[base+1*k : base+2*k]
		r2 := a[base+2*k : base+3*k]
		r3 := a[base+3*k : base+4*k]
		r4 := a[base+4*k : base+5*k]
		r5 := a[base+5*k : base+6*k]
		for kk := 0; kk < k; kk++ {
			d := dst[kk*gemmMr : kk*gemmMr+gemmMr : kk*gemmMr+gemmMr]
			d[0], d[1], d[2] = r0[kk], r1[kk], r2[kk]
			d[3], d[4], d[5] = r3[kk], r4[kk], r5[kk]
		}
	}
}
