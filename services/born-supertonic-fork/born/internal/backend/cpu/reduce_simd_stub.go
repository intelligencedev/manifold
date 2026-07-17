//go:build !amd64 || !goexperiment.simd

package cpu

// These functions are nil when SIMD is unavailable (non-amd64 or built
// without GOEXPERIMENT=simd).  They fall back to the scalar
// loop when nil.
var simdSumFloat32 func(dst, src []float32)
var simdSumFloat64 func(dst, src []float64)
var simdSumInt32 func(dst, src []int32)
var simdSumInt64 func(dst, src []int64)
