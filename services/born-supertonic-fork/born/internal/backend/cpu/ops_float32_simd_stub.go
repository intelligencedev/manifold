//go:build !amd64 || !goexperiment.simd

package cpu

// These functions are nil when SIMD is unavailable (non-amd64 or built
// without GOEXPERIMENT=simd).  They fall back to the scalar
// loop when nil.
var simdAddInplaceFloat32 func(a, b []float32)
var simdSubInplaceFloat32 func(a, b []float32)
var simdMulInplaceFloat32 func(a, b []float32)
var simdDivInplaceFloat32 func(a, b []float32)

var simdAddVectorizedFloat32 func(dst, a, b []float32)
var simdSubVectorizedFloat32 func(dst, a, b []float32)
var simdMulVectorizedFloat32 func(dst, a, b []float32)
var simdDivVectorizedFloat32 func(dst, a, b []float32)
