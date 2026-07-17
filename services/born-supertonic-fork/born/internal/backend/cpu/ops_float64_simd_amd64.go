//go:build amd64 && goexperiment.simd

package cpu

import "simd/archsimd"

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdAddInplaceFloat64 func(a, b []float64)
var simdSubInplaceFloat64 func(a, b []float64)
var simdMulInplaceFloat64 func(a, b []float64)
var simdDivInplaceFloat64 func(a, b []float64)

var simdAddVectorizedFloat64 func(dst, a, b []float64)
var simdSubVectorizedFloat64 func(dst, a, b []float64)
var simdMulVectorizedFloat64 func(dst, a, b []float64)
var simdDivVectorizedFloat64 func(dst, a, b []float64)

func init() {
	if archsimd.X86.AVX() {
		simdAddInplaceFloat64 = avxAddInplaceFloat64
		simdSubInplaceFloat64 = avxSubInplaceFloat64
		simdMulInplaceFloat64 = avxMulInplaceFloat64
		simdDivInplaceFloat64 = avxDivInplaceFloat64

		simdAddVectorizedFloat64 = avxAddVectorizedFloat64
		simdSubVectorizedFloat64 = avxSubVectorizedFloat64
		simdMulVectorizedFloat64 = avxMulVectorizedFloat64
		simdDivVectorizedFloat64 = avxDivVectorizedFloat64
	}

	if archsimd.X86.AVX512() {
		simdAddInplaceFloat64 = avx512AddInplaceFloat64
		simdSubInplaceFloat64 = avx512SubInplaceFloat64
		simdMulInplaceFloat64 = avx512MulInplaceFloat64
		simdDivInplaceFloat64 = avx512DivInplaceFloat64

		simdAddVectorizedFloat64 = avx512AddVectorizedFloat64
		simdSubVectorizedFloat64 = avx512SubVectorizedFloat64
		simdMulVectorizedFloat64 = avx512MulVectorizedFloat64
		simdDivVectorizedFloat64 = avx512DivVectorizedFloat64
	}
}

// avxAddInplaceFloat64 computes a[i] += b[i] using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxAddInplaceFloat64(a, b []float64) {
	n := len(a)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadFloat64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadFloat64x4Slice(b[i : i+4])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(a[i : i+4])
	}
	for ; i < n; i++ {
		a[i] += b[i]
	}
}

// avx512AddInplaceFloat64 computes a[i] += b[i] using AVX-512 (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512AddInplaceFloat64(a, b []float64) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat64x8Slice(b[i : i+8])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] += b[i]
	}
}

// avxSubInplaceFloat64 computes a[i] -= b[i] using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxSubInplaceFloat64(a, b []float64) {
	n := len(a)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadFloat64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadFloat64x4Slice(b[i : i+4])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(a[i : i+4])
	}
	for ; i < n; i++ {
		a[i] -= b[i]
	}
}

// avx512SubInplaceFloat64 computes a[i] -= b[i] using AVX-512 (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512SubInplaceFloat64(a, b []float64) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat64x8Slice(b[i : i+8])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] -= b[i]
	}
}

// avxMulInplaceFloat64 computes a[i] *= b[i] using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxMulInplaceFloat64(a, b []float64) {
	n := len(a)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadFloat64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadFloat64x4Slice(b[i : i+4])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(a[i : i+4])
	}
	for ; i < n; i++ {
		a[i] *= b[i]
	}
}

// avx512MulInplaceFloat64 computes a[i] *= b[i] using AVX-512 (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512MulInplaceFloat64(a, b []float64) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat64x8Slice(b[i : i+8])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] *= b[i]
	}
}

// avxDivInplaceFloat64 computes a[i] /= b[i] using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxDivInplaceFloat64(a, b []float64) {
	n := len(a)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadFloat64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadFloat64x4Slice(b[i : i+4])
		divLoaded := aLoaded.Div(bLoaded)
		divLoaded.StoreSlice(a[i : i+4])
	}
	for ; i < n; i++ {
		a[i] /= b[i]
	}
}

// avx512DivInplaceFloat64 computes a[i] /= b[i] using AVX-512 (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512DivInplaceFloat64(a, b []float64) {
	n := len(a)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat64x8Slice(b[i : i+8])
		divLoaded := aLoaded.Div(bLoaded)
		divLoaded.StoreSlice(a[i : i+8])
	}
	for ; i < n; i++ {
		a[i] /= b[i]
	}
}

// avxAddVectorizedFloat64 computes dst[i] = a[i] + b[i] using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxAddVectorizedFloat64(dst, a, b []float64) {
	n := len(dst)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadFloat64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadFloat64x4Slice(b[i : i+4])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(dst[i : i+4])
	}
	for ; i < n; i++ {
		dst[i] = a[i] + b[i]
	}
}

// avx512AddVectorizedFloat64 computes dst[i] = a[i] + b[i] using AVX-512 (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512AddVectorizedFloat64(dst, a, b []float64) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat64x8Slice(b[i : i+8])
		sumLoaded := aLoaded.Add(bLoaded)
		sumLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] + b[i]
	}
}

// avxSubVectorizedFloat64 computes dst[i] = a[i] - b[i] using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxSubVectorizedFloat64(dst, a, b []float64) {
	n := len(dst)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadFloat64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadFloat64x4Slice(b[i : i+4])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(dst[i : i+4])
	}
	for ; i < n; i++ {
		dst[i] = a[i] - b[i]
	}
}

// avx512SubVectorizedFloat64 computes dst[i] = a[i] - b[i] using AVX-512 (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512SubVectorizedFloat64(dst, a, b []float64) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat64x8Slice(b[i : i+8])
		subLoaded := aLoaded.Sub(bLoaded)
		subLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] - b[i]
	}
}

// avxMulVectorizedFloat64 computes dst[i] = a[i] * b[i] using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxMulVectorizedFloat64(dst, a, b []float64) {
	n := len(dst)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadFloat64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadFloat64x4Slice(b[i : i+4])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(dst[i : i+4])
	}
	for ; i < n; i++ {
		dst[i] = a[i] * b[i]
	}
}

// avx512MulVectorizedFloat64 computes dst[i] = a[i] * b[i] using AVX-512 (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512MulVectorizedFloat64(dst, a, b []float64) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat64x8Slice(b[i : i+8])
		mulLoaded := aLoaded.Mul(bLoaded)
		mulLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] * b[i]
	}
}

// avxDivVectorizedFloat64 computes dst[i] = a[i] / b[i] using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxDivVectorizedFloat64(dst, a, b []float64) {
	n := len(dst)
	i := 0
	for ; i+4 <= n; i += 4 {
		aLoaded := archsimd.LoadFloat64x4Slice(a[i : i+4])
		bLoaded := archsimd.LoadFloat64x4Slice(b[i : i+4])
		divLoaded := aLoaded.Div(bLoaded)
		divLoaded.StoreSlice(dst[i : i+4])
	}
	for ; i < n; i++ {
		dst[i] = a[i] / b[i]
	}
}

// avx512DivVectorizedFloat64 computes dst[i] = a[i] / b[i] using AVX-512 (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512DivVectorizedFloat64(dst, a, b []float64) {
	n := len(dst)
	i := 0
	for ; i+8 <= n; i += 8 {
		aLoaded := archsimd.LoadFloat64x8Slice(a[i : i+8])
		bLoaded := archsimd.LoadFloat64x8Slice(b[i : i+8])
		divLoaded := aLoaded.Div(bLoaded)
		divLoaded.StoreSlice(dst[i : i+8])
	}
	for ; i < n; i++ {
		dst[i] = a[i] / b[i]
	}
}
