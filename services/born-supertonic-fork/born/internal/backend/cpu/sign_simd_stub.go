//go:build !amd64 || !goexperiment.simd

package cpu

// These functions are nil when SIMD is unavailable (non-amd64 or built
// without GOEXPERIMENT=simd).  They fall back to the scalar
// loop when nil.
var simdSignFloat32 func(dst, src []float32)
var simdSignFloat64 func(dst, src []float64)
var simdSignInt32 func(dst, src []int32)
var simdSignInt64 func(dst, src []int64)
var simdSignUint8 func(dst, src []uint8)
