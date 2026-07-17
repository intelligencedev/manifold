//go:build amd64 && goexperiment.simd

package cpu

import "simd/archsimd"

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdAddInplaceFloat32 func(a, b []float32)
var simdSubInplaceFloat32 func(a, b []float32)
var simdMulInplaceFloat32 func(a, b []float32)
var simdDivInplaceFloat32 func(a, b []float32)

var simdAddVectorizedFloat32 func(dst, a, b []float32)
var simdSubVectorizedFloat32 func(dst, a, b []float32)
var simdMulVectorizedFloat32 func(dst, a, b []float32)
var simdDivVectorizedFloat32 func(dst, a, b []float32)

func init() {
	if archsimd.X86.AVX() {
		simdAddInplaceFloat32 = avxAddInplaceFloat32
		simdSubInplaceFloat32 = avxSubInplaceFloat32
		simdMulInplaceFloat32 = avxMulInplaceFloat32
		simdDivInplaceFloat32 = avxDivInplaceFloat32

		simdAddVectorizedFloat32 = avxAddVectorizedFloat32
		simdSubVectorizedFloat32 = avxSubVectorizedFloat32
		simdMulVectorizedFloat32 = avxMulVectorizedFloat32
		simdDivVectorizedFloat32 = avxDivVectorizedFloat32
	}
	if archsimd.X86.AVX512() {
		simdAddInplaceFloat32 = avx512AddInplaceFloat32
		simdSubInplaceFloat32 = avx512SubInplaceFloat32
		simdMulInplaceFloat32 = avx512MulInplaceFloat32
		simdDivInplaceFloat32 = avx512DivInplaceFloat32

		simdAddVectorizedFloat32 = avx512AddVectorizedFloat32
		simdSubVectorizedFloat32 = avx512SubVectorizedFloat32
		simdMulVectorizedFloat32 = avx512MulVectorizedFloat32
		simdDivVectorizedFloat32 = avx512DivVectorizedFloat32
	}
}

// avxAddInplaceFloat32 computes a[i] += b[i] using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxAddInplaceFloat32(a, b []float32) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat32x8Slice(b[i : i+8])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] += b[i]
	}
}

// avx512AddInplaceFloat32 computes a[i] += b[i] using AVX-512 (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512AddInplaceFloat32(a, b []float32) {
	n := len(a)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadFloat32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadFloat32x16Slice(b[i : i+16])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(a[i : i+16])
	}
	for ; i < n; i++ {
		a[i] += b[i]
	}
}

// avxSubInplaceFloat32 computes a[i] -= b[i] using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxSubInplaceFloat32(a, b []float32) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat32x8Slice(b[i : i+8])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] -= b[i]
	}
}

// avx512SubInplaceFloat32 computes a[i] -= b[i] using AVX-512 (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512SubInplaceFloat32(a, b []float32) {
	n := len(a)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadFloat32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadFloat32x16Slice(b[i : i+16])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(a[i : i+16])
	}
	for ; i < n; i++ {
		a[i] -= b[i]
	}
}

// avxMulInplaceFloat32 computes a[i] *= b[i] using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxMulInplaceFloat32(a, b []float32) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat32x8Slice(b[i : i+8])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] *= b[i]
	}
}

// avx512MulInplaceFloat32 computes a[i] *= b[i] using AVX-512 (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512MulInplaceFloat32(a, b []float32) {
	n := len(a)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadFloat32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadFloat32x16Slice(b[i : i+16])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(a[i : i+16])
	}
	for ; i < n; i++ {
		a[i] *= b[i]
	}
}

// avxDivInplaceFloat32 computes a[i] /= b[i] using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxDivInplaceFloat32(a, b []float32) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat32x8Slice(b[i : i+8])
		divLoaded := aLoaded.Div(bLoaded)
		divLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] /= b[i]
	}
}

// avx512DivInplaceFloat32 computes a[i] /= b[i] using AVX-512 (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512DivInplaceFloat32(a, b []float32) {
	n := len(a)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadFloat32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadFloat32x16Slice(b[i : i+16])
		divLoaded := aLoaded.Div(bLoaded)
		divLoaded.StoreSlice(a[i : i+16])
	}
	for ; i < n; i++ {
		a[i] /= b[i]
	}
}

// avxAddVectorizedFloat32 computes dst[i] = a[i] + b[i] using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxAddVectorizedFloat32(dst, a, b []float32) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat32x8Slice(b[i : i+8])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] + b[i]
	}
}

// avx512AddVectorizedFloat32 computes dst[i] = a[i] + b[i] using AVX-512 (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512AddVectorizedFloat32(dst, a, b []float32) {
	n := len(dst)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadFloat32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadFloat32x16Slice(b[i : i+16])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(dst[i : i+16])
	}
	for ; i < n; i++ {
		dst[i] = a[i] + b[i]
	}
}

// avxSubVectorizedFloat32 computes dst[i] = a[i] - b[i] using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxSubVectorizedFloat32(dst, a, b []float32) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat32x8Slice(b[i : i+8])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] - b[i]
	}
}

// avx512SubVectorizedFloat32 computes dst[i] = a[i] - b[i] using AVX-512 (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512SubVectorizedFloat32(dst, a, b []float32) {
	n := len(dst)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadFloat32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadFloat32x16Slice(b[i : i+16])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(dst[i : i+16])
	}
	for ; i < n; i++ {
		dst[i] = a[i] - b[i]
	}
}

// avxMulVectorizedFloat32 computes dst[i] = a[i] * b[i] using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxMulVectorizedFloat32(dst, a, b []float32) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat32x8Slice(b[i : i+8])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] * b[i]
	}
}

// avx512MulVectorizedFloat32 computes dst[i] = a[i] * b[i] using AVX-512 (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512MulVectorizedFloat32(dst, a, b []float32) {
	n := len(dst)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadFloat32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadFloat32x16Slice(b[i : i+16])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(dst[i : i+16])
	}
	for ; i < n; i++ {
		dst[i] = a[i] * b[i]
	}
}

// avxDivVectorizedFloat32 computes dst[i] = a[i] / b[i] using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxDivVectorizedFloat32(dst, a, b []float32) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat32x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat32x8Slice(b[i : i+8])
		divLoaded := aLoaded.Div(bLoaded)
		divLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] / b[i]
	}
}

// avx512DivVectorizedFloat32 computes dst[i] = a[i] / b[i] using AVX-512 (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512DivVectorizedFloat32(dst, a, b []float32) {
	n := len(dst)
	i := 0
	for ; i+16 <= n; i += 16 {
		aLoaded := archsimd.LoadFloat32x16Slice(a[i : i+16])
		bLoaded := archsimd.LoadFloat32x16Slice(b[i : i+16])
		divLoaded := aLoaded.Div(bLoaded)
		divLoaded.StoreSlice(dst[i : i+16])
	}
	for ; i < n; i++ {
		dst[i] = a[i] / b[i]
	}
}
