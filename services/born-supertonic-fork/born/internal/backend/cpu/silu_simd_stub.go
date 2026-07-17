//go:build !amd64 || !goexperiment.simd

package cpu

// These functions are nil when SIMD is unavailable (non-amd64 or built
// without GOEXPERIMENT=simd).  They fall back to the scalar
// loop when nil.
var simdSiluFloat32 func(dst, src []float32)
var simdSiluFloat64 func(dst, src []float64)
