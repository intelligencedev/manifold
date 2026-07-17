//go:build !amd64 || !goexperiment.simd

package cpu

// These functions are nil when SIMD is unavailable (non-amd64 or built
// without GOEXPERIMENT=simd).  They fall back to the scalar
// loop when nil.
//
// Note: SIMD integer division is not supported by simd/archsimd,
// the scalar fallback is used for all division operations.
var simdAddInplaceInt64 func(a, b []int64)
var simdSubInplaceInt64 func(a, b []int64)
var simdMulInplaceInt64 func(a, b []int64)

var simdAddVectorizedInt64 func(dst, a, b []int64)
var simdSubVectorizedInt64 func(dst, a, b []int64)
var simdMulVectorizedInt64 func(dst, a, b []int64)
