//go:build !amd64 || !goexperiment.simd

package cpu

// simdMicroKernelF32 is nil when SIMD is unavailable (non-amd64 or built
// without GOEXPERIMENT=simd).  matmulMicroKernelF32 falls back to the scalar
// loop when this is nil.
var simdMicroKernelF32 func(c, a, b []float32, k, n, ii, iEnd, kk, kEnd, jj, jEnd int)
