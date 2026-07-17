//go:build amd64 && goexperiment.simd

package cpu

import "simd/archsimd"

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdAddInplaceInt32 func(a, b []int32)
var simdSubInplaceInt32 func(a, b []int32)
var simdMulInplaceInt32 func(a, b []int32)

var simdAddVectorizedInt32 func(dst, a, b []int32)
var simdSubVectorizedInt32 func(dst, a, b []int32)
var simdMulVectorizedInt32 func(dst, a, b []int32)

func init() {
	if archsimd.X86.AVX2() {
		simdAddInplaceInt32 = avx2AddInplaceInt32
		simdSubInplaceInt32 = avx2SubInplaceInt32
		simdMulInplaceInt32 = avx2MulInplaceInt32

		simdAddVectorizedInt32 = avx2AddVectorizedInt32
		simdSubVectorizedInt32 = avx2SubVectorizedInt32
		simdMulVectorizedInt32 = avx2MulVectorizedInt32
	}
	if archsimd.X86.AVX512() {
		simdAddInplaceInt32 = avx512AddInplaceInt32
		simdSubInplaceInt32 = avx512SubInplaceInt32
		simdMulInplaceInt32 = avx512MulInplaceInt32

		simdAddVectorizedInt32 = avx512AddVectorizedInt32
		simdSubVectorizedInt32 = avx512SubVectorizedInt32
		simdMulVectorizedInt32 = avx512MulVectorizedInt32
	}
}

// avx2AddInplaceInt32 computes a[i] += b[i] using AVX2 (256-bit, 8 int32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx2AddInplaceInt32(a, b []int32) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt32x8Slice(b[i : i+8])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] += b[i]
	}
}

// avx512AddInplaceInt32 computes a[i] += b[i] using AVX-512 (512-bit, 16 int32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512AddInplaceInt32(a, b []int32) {
	n := len(a)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadInt32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadInt32x16Slice(b[i : i+16])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(a[i : i+16])
	}
	for ; i < n; i++ {
		a[i] += b[i]
	}
}

// avx2SubInplaceInt32 computes a[i] -= b[i] using AVX2 (256-bit, 8 int32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx2SubInplaceInt32(a, b []int32) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt32x8Slice(b[i : i+8])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] -= b[i]
	}
}

// avx512SubInplaceInt32 computes a[i] -= b[i] using AVX-512 (512-bit, 16 int32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512SubInplaceInt32(a, b []int32) {
	n := len(a)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadInt32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadInt32x16Slice(b[i : i+16])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(a[i : i+16])
	}
	for ; i < n; i++ {
		a[i] -= b[i]
	}
}

// avx2MulInplaceInt32 computes a[i] *= b[i] using AVX2 (256-bit, 8 int32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx2MulInplaceInt32(a, b []int32) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt32x8Slice(b[i : i+8])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] *= b[i]
	}
}

// avx512MulInplaceInt32 computes a[i] *= b[i] using AVX-512 (512-bit, 16 int32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512MulInplaceInt32(a, b []int32) {
	n := len(a)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadInt32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadInt32x16Slice(b[i : i+16])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(a[i : i+16])
	}
	for ; i < n; i++ {
		a[i] *= b[i]
	}
}

// avx2AddVectorizedInt32 computes dst[i] = a[i] + b[i] using AVX2 (256-bit, 8 int32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx2AddVectorizedInt32(dst, a, b []int32) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt32x8Slice(b[i : i+8])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] + b[i]
	}
}

// avx512AddVectorizedInt32 computes dst[i] = a[i] + b[i] using AVX-512 (512-bit, 16 int32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512AddVectorizedInt32(dst, a, b []int32) {
	n := len(dst)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadInt32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadInt32x16Slice(b[i : i+16])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(dst[i : i+16])
	}
	for ; i < n; i++ {
		dst[i] = a[i] + b[i]
	}
}

// avx2SubVectorizedInt32 computes dst[i] = a[i] - b[i] using AVX2 (256-bit, 8 int32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx2SubVectorizedInt32(dst, a, b []int32) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt32x8Slice(b[i : i+8])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] - b[i]
	}
}

// avx512SubVectorizedInt32 computes dst[i] = a[i] - b[i] using AVX-512 (512-bit, 16 int32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512SubVectorizedInt32(dst, a, b []int32) {
	n := len(dst)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadInt32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadInt32x16Slice(b[i : i+16])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(dst[i : i+16])
	}
	for ; i < n; i++ {
		dst[i] = a[i] - b[i]
	}
}

// avx2MulVectorizedInt32 computes dst[i] = a[i] * b[i] using AVX2 (256-bit, 8 int32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx2MulVectorizedInt32(dst, a, b []int32) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadInt32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadInt32x8Slice(b[i : i+8])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] * b[i]
	}
}

// avx512MulVectorizedInt32 computes dst[i] = a[i] * b[i] using AVX-512 (512-bit, 16 int32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512MulVectorizedInt32(dst, a, b []int32) {
	n := len(dst)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadInt32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadInt32x16Slice(b[i : i+16])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(dst[i : i+16])
	}
	for ; i < n; i++ {
		dst[i] = a[i] * b[i]
	}
}
