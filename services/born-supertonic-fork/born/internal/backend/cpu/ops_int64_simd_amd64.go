//go:build amd64 && goexperiment.simd

package cpu

import "simd/archsimd"

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdAddInplaceInt64 func(a, b []int64)
var simdSubInplaceInt64 func(a, b []int64)
var simdMulInplaceInt64 func(a, b []int64)

var simdAddVectorizedInt64 func(dst, a, b []int64)
var simdSubVectorizedInt64 func(dst, a, b []int64)
var simdMulVectorizedInt64 func(dst, a, b []int64)

func init() {
	if archsimd.X86.AVX2() {
		simdAddInplaceInt64 = avx2AddInplaceInt64
		simdSubInplaceInt64 = avx2SubInplaceInt64

		simdAddVectorizedInt64 = avx2AddVectorizedInt64
		simdSubVectorizedInt64 = avx2SubVectorizedInt64
	}
	if archsimd.X86.AVX512() {
		simdAddInplaceInt64 = avx512AddInplaceInt64
		simdSubInplaceInt64 = avx512SubInplaceInt64
		simdMulInplaceInt64 = avx512MulInplaceInt64

		simdAddVectorizedInt64 = avx512AddVectorizedInt64
		simdSubVectorizedInt64 = avx512SubVectorizedInt64
		simdMulVectorizedInt64 = avx512MulVectorizedInt64
	}
}

// avx2AddInplaceInt64 computes a[i] += b[i] using AVX2 (256-bit, 4 int64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avx2AddInplaceInt64(a, b []int64) {
	n := len(a)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadInt64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadInt64x4Slice(b[i : i+4])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(a[i : i+4])
	}
	for ; i < n; i++ {
		a[i] += b[i]
	}
}

// avx512AddInplaceInt64 computes a[i] += b[i] using AVX-512 (512-bit, 8 int64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512AddInplaceInt64(a, b []int64) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt64x8Slice(b[i : i+8])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] += b[i]
	}
}

// avx2SubInplaceInt64 computes a[i] -= b[i] using AVX2 (256-bit, 4 int64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avx2SubInplaceInt64(a, b []int64) {
	n := len(a)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadInt64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadInt64x4Slice(b[i : i+4])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(a[i : i+4])
	}
	for ; i < n; i++ {
		a[i] -= b[i]
	}
}

// avx512SubInplaceInt64 computes a[i] -= b[i] using AVX-512 (512-bit, 8 int64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512SubInplaceInt64(a, b []int64) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt64x8Slice(b[i : i+8])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] -= b[i]
	}
}

// avx512MulInplaceInt64 computes a[i] *= b[i] using AVX-512 (512-bit, 8 int64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512MulInplaceInt64(a, b []int64) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt64x8Slice(b[i : i+8])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] *= b[i]
	}
}

// avx2AddVectorizedInt64 computes dst[i] = a[i] + b[i] using AVX2 (256-bit, 4 int64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avx2AddVectorizedInt64(dst, a, b []int64) {
	n := len(dst)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadInt64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadInt64x4Slice(b[i : i+4])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(dst[i : i+4])
	}
	for ; i < n; i++ {
		dst[i] = a[i] + b[i]
	}
}

// avx512AddVectorizedInt64 computes dst[i] = a[i] + b[i] using AVX-512 (512-bit, 8 int64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512AddVectorizedInt64(dst, a, b []int64) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt64x8Slice(b[i : i+8])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] + b[i]
	}
}

// avx2SubVectorizedInt64 computes dst[i] = a[i] - b[i] using AVX2 (256-bit, 4 int64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avx2SubVectorizedInt64(dst, a, b []int64) {
	n := len(dst)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadInt64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadInt64x4Slice(b[i : i+4])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(dst[i : i+4])
	}
	for ; i < n; i++ {
		dst[i] = a[i] - b[i]
	}
}

// avx512SubVectorizedInt64 computes dst[i] = a[i] - b[i] using AVX-512 (512-bit, 8 int64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512SubVectorizedInt64(dst, a, b []int64) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt64x8Slice(b[i : i+8])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] - b[i]
	}
}

// avx512MulVectorizedInt64 computes dst[i] = a[i] * b[i] using AVX-512 (512-bit, 8 int64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512MulVectorizedInt64(dst, a, b []int64) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt64x8Slice(b[i : i+8])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] * b[i]
	}
}
