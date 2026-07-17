//go:build amd64 && goexperiment.simd

package cpu

import "simd/archsimd"

// simdMicroKernelF32 holds the AVX2 micro-kernel when available.
// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdMicroKernelF32 func(c, a, b []float32, k, n, ii, iEnd, kk, kEnd, jj, jEnd int)

func init() {
	if archsimd.X86.AVX2() {
		simdMicroKernelF32 = avx2MicroKernelF32
	}
}

// avx2MicroKernelF32 is the AVX2 (256-bit, 8 float32/vector) replacement for
// the scalar matmulMicroKernelF32.  It accumulates the block product
//
//	A[ii:iEnd, kk:kEnd] × B[kk:kEnd, jj:jEnd]
//
// into C[ii:iEnd, jj:jEnd] using register blocking:
//   - 4 i-rows per outer iteration (keeps 8 accumulators alive across the k-loop)
//   - 2 × Float32x8 (16 floats) per j-tile, processed in the vectorised fast path
//   - 1 × Float32x8 (8 floats) mid path, then scalar tail for the final 0-7 elements
//
// LoadFloat32x8Slice / StoreSlice operate directly on []float32 sub-slices, so
// no unsafe.Pointer casting is required (unlike the GoMLX AVX-512 reference which
// must cast to *[16]float32).
//
// MulAdd semantics: receiver.MulAdd(y, z) == receiver*y + z
// So  aVec.MulAdd(bVec, accum) == aVal*bVec + accum  (i.e. accum += aVal*bVec).
func avx2MicroKernelF32(c, a, b []float32, k, n, ii, iEnd, kk, kEnd, jj, jEnd int) {
	// ----------------------------------------------------------------
	// Fast path: process 4 i-rows at a time with 2×Float32x8 j-tiles.
	// This keeps 8 accumulator registers (4 rows × 2 vectors) occupied
	// across the k-loop, which is the key to hiding FMA latency on AVX2.
	// ----------------------------------------------------------------
	i := ii
	for ; i+4 <= iEnd; i += 4 {
		for kIdx := kk; kIdx < kEnd; kIdx++ {
			// Hoist the four A scalars out of the j-loop and broadcast
			// each into a full 8-lane vector.
			aVec0 := archsimd.BroadcastFloat32x8(a[i*k+kIdx])
			aVec1 := archsimd.BroadcastFloat32x8(a[(i+1)*k+kIdx])
			aVec2 := archsimd.BroadcastFloat32x8(a[(i+2)*k+kIdx])
			aVec3 := archsimd.BroadcastFloat32x8(a[(i+3)*k+kIdx])

			// Row base pointers into C and B for this kIdx.
			c0Base := i * n
			c1Base := (i + 1) * n
			c2Base := (i + 2) * n
			c3Base := (i + 3) * n
			bBase := kIdx * n

			j := jj

			// --- 16-wide tile (2 × Float32x8) ---
			for ; j+16 <= jEnd; j += 16 {
				bVec0 := archsimd.LoadFloat32x8Slice(b[bBase+j:])
				bVec1 := archsimd.LoadFloat32x8Slice(b[bBase+j+8:])

				// Row 0
				cVec00 := archsimd.LoadFloat32x8Slice(c[c0Base+j:])
				cVec01 := archsimd.LoadFloat32x8Slice(c[c0Base+j+8:])
				cVec00 = aVec0.MulAdd(bVec0, cVec00)
				cVec01 = aVec0.MulAdd(bVec1, cVec01)
				cVec00.StoreSlice(c[c0Base+j:])
				cVec01.StoreSlice(c[c0Base+j+8:])

				// Row 1
				cVec10 := archsimd.LoadFloat32x8Slice(c[c1Base+j:])
				cVec11 := archsimd.LoadFloat32x8Slice(c[c1Base+j+8:])
				cVec10 = aVec1.MulAdd(bVec0, cVec10)
				cVec11 = aVec1.MulAdd(bVec1, cVec11)
				cVec10.StoreSlice(c[c1Base+j:])
				cVec11.StoreSlice(c[c1Base+j+8:])

				// Row 2
				cVec20 := archsimd.LoadFloat32x8Slice(c[c2Base+j:])
				cVec21 := archsimd.LoadFloat32x8Slice(c[c2Base+j+8:])
				cVec20 = aVec2.MulAdd(bVec0, cVec20)
				cVec21 = aVec2.MulAdd(bVec1, cVec21)
				cVec20.StoreSlice(c[c2Base+j:])
				cVec21.StoreSlice(c[c2Base+j+8:])

				// Row 3
				cVec30 := archsimd.LoadFloat32x8Slice(c[c3Base+j:])
				cVec31 := archsimd.LoadFloat32x8Slice(c[c3Base+j+8:])
				cVec30 = aVec3.MulAdd(bVec0, cVec30)
				cVec31 = aVec3.MulAdd(bVec1, cVec31)
				cVec30.StoreSlice(c[c3Base+j:])
				cVec31.StoreSlice(c[c3Base+j+8:])
			}

			// --- 8-wide tile (1 × Float32x8) ---
			if j+8 <= jEnd {
				bVec := archsimd.LoadFloat32x8Slice(b[bBase+j:])

				cVec0 := archsimd.LoadFloat32x8Slice(c[c0Base+j:])
				cVec0 = aVec0.MulAdd(bVec, cVec0)
				cVec0.StoreSlice(c[c0Base+j:])

				cVec1 := archsimd.LoadFloat32x8Slice(c[c1Base+j:])
				cVec1 = aVec1.MulAdd(bVec, cVec1)
				cVec1.StoreSlice(c[c1Base+j:])

				cVec2 := archsimd.LoadFloat32x8Slice(c[c2Base+j:])
				cVec2 = aVec2.MulAdd(bVec, cVec2)
				cVec2.StoreSlice(c[c2Base+j:])

				cVec3 := archsimd.LoadFloat32x8Slice(c[c3Base+j:])
				cVec3 = aVec3.MulAdd(bVec, cVec3)
				cVec3.StoreSlice(c[c3Base+j:])

				j += 8
			}

			// --- Scalar tail (0-7 elements) ---
			aVal0 := a[i*k+kIdx]
			aVal1 := a[(i+1)*k+kIdx]
			aVal2 := a[(i+2)*k+kIdx]
			aVal3 := a[(i+3)*k+kIdx]
			for ; j < jEnd; j++ {
				bVal := b[bBase+j]
				c[c0Base+j] += aVal0 * bVal
				c[c1Base+j] += aVal1 * bVal
				c[c2Base+j] += aVal2 * bVal
				c[c3Base+j] += aVal3 * bVal
			}
		}
	}

	// ----------------------------------------------------------------
	// Row fringe: handle remaining 1-3 rows with the same j-tiling
	// but without the 4-row register blocking above.
	// ----------------------------------------------------------------
	for ; i < iEnd; i++ {
		for kIdx := kk; kIdx < kEnd; kIdx++ {
			aVec := archsimd.BroadcastFloat32x8(a[i*k+kIdx])
			cBase := i * n
			bBase := kIdx * n

			j := jj

			// 16-wide tile
			for ; j+16 <= jEnd; j += 16 {
				bVec0 := archsimd.LoadFloat32x8Slice(b[bBase+j:])
				bVec1 := archsimd.LoadFloat32x8Slice(b[bBase+j+8:])
				cVec0 := archsimd.LoadFloat32x8Slice(c[cBase+j:])
				cVec1 := archsimd.LoadFloat32x8Slice(c[cBase+j+8:])
				cVec0 = aVec.MulAdd(bVec0, cVec0)
				cVec1 = aVec.MulAdd(bVec1, cVec1)
				cVec0.StoreSlice(c[cBase+j:])
				cVec1.StoreSlice(c[cBase+j+8:])
			}

			// 8-wide tile
			if j+8 <= jEnd {
				bVec := archsimd.LoadFloat32x8Slice(b[bBase+j:])
				cVec := archsimd.LoadFloat32x8Slice(c[cBase+j:])
				cVec = aVec.MulAdd(bVec, cVec)
				cVec.StoreSlice(c[cBase+j:])
				j += 8
			}

			// Scalar tail
			aVal := a[i*k+kIdx]
			for ; j < jEnd; j++ {
				c[cBase+j] += aVal * b[bBase+j]
			}
		}
	}
}
