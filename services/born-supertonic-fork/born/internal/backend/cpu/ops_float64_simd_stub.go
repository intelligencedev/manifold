//go:build !amd64 || !goexperiment.simd

package cpu

// These functions are nil when SIMD is unavailable (non-amd64 or built
// without GOEXPERIMENT=simd).  They fall back to the scalar
// loop when nil.
var simdAddInplaceFloat64 func(a, b []float64)
var simdSubInplaceFloat64 func(a, b []float64)
var simdMulInplaceFloat64 func(a, b []float64)
var simdDivInplaceFloat64 func(a, b []float64)

var simdAddVectorizedFloat64 func(dst, a, b []float64)
var simdSubVectorizedFloat64 func(dst, a, b []float64)
var simdMulVectorizedFloat64 func(dst, a, b []float64)
var simdDivVectorizedFloat64 func(dst, a, b []float64)
