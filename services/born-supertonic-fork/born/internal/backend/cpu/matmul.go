package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Block sizes tuned to fit within L1 cache.
//
// float32: 64×64 × 4 bytes = 16 KB — fits a typical 32 KB L1 data cache.
// float64: 32×32 × 8 bytes =  8 KB — same budget, halved dimensions.
const (
	blockSizeF32 = 64
	blockSizeF64 = 32

	// blockThreshold is the minimum number of multiply-accumulate operations before
	// cache-tiled blocking pays off. Below this the loop setup overhead exceeds the
	// cache-miss savings. Value derived from the GoMLX reference (4 MB / 4 bytes).
	blockThreshold = 64 * 64 * 64 // ~262 144 ops
)

// MatMul performs matrix multiplication.
// For 2D tensors: (M, K) @ (K, N) -> (M, N)
// Uses cache-tiled blocked GEMM for large matrices; falls back to the naive
// triple-loop for small ones where blocking overhead exceeds the benefit.
func (cpu *CPUBackend) MatMul(a, b *tensor.RawTensor) *tensor.RawTensor {
	aShape := a.Shape()
	bShape := b.Shape()

	// Validate dimensions.
	if len(aShape) != 2 || len(bShape) != 2 {
		panic(fmt.Sprintf("matmul: only 2D tensors supported, got %dD and %dD", len(aShape), len(bShape)))
	}

	m, k := aShape[0], aShape[1]
	kAlt, n := bShape[0], bShape[1]

	if k != kAlt {
		panic(fmt.Sprintf("matmul: shape mismatch [%d,%d] @ [%d,%d]", m, k, kAlt, n))
	}

	// Create result tensor.
	result, err := tensor.NewRaw(tensor.Shape{m, n}, a.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("matmul: failed to create result tensor: %v", err))
	}

	// Dispatch to type-specific implementation.
	switch a.DType() {
	case tensor.Float32:
		matmulFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32(), m, k, n)
	case tensor.Float64:
		matmulFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64(), m, k, n)
	case tensor.Int32:
		matmulInt32(result.AsInt32(), a.AsInt32(), b.AsInt32(), m, k, n)
	case tensor.Int64:
		matmulInt64(result.AsInt64(), a.AsInt64(), b.AsInt64(), m, k, n)
	default:
		panic(fmt.Sprintf("matmul: unsupported dtype %s", a.DType()))
	}

	return result
}

// matmulFloat32 multiplies A (m×k) by B (k×n) and writes the result into C (m×n).
//
// C is OVERWRITTEN, not accumulated into: the function zeroes c up front, so
// callers may pass a dirty buffer and rely on getting a pure A×B result without
// pre-zeroing it themselves (pointwiseConvFloat32 depends on this). Any future
// accumulate-mode variant must be a separate function, not a behavior change
// here, or that contract breaks silently.
//
// For matrices large enough to benefit from cache locality (m*k*n >= blockThreshold)
// a three-level cache-tiled algorithm is used: outer loops stride in blocks that fit
// the L1 cache, while the micro-kernel accumulates a block with sequential B access.
// Smaller matrices fall back to the naive triple-loop. When the vendored-SIMD GEMM
// kernel is wired in (gemmF32 != nil; set at init on AVX2+FMA CPUs, see
// matmul_gemm_amd64.go), large multiplications are routed there instead, overwriting
// C directly.
//
// The caller must pass c pre-capped to exactly m*n elements.
func matmulFloat32(c, a, b []float32, m, k, n int) {
	// Vendored-SIMD GEMM fast path. It overwrites C directly, so it must run before
	// the scalar zero-init below. Narrow shapes (n < one full column tile) stay on the
	// scalar path, where cache-tiled blocking beats the kernel's naive column remainder.
	if gemmF32 != nil && n >= gemmMinCols && m*k*n >= blockThreshold {
		gemmF32(c, a, b, m, k, n)
		return
	}

	for i := range c {
		c[i] = 0
	}

	if m*k*n < blockThreshold {
		matmulNaiveFloat32(c, a, b, m, k, n)
		return
	}

	for ii := 0; ii < m; ii += blockSizeF32 {
		iEnd := min(ii+blockSizeF32, m)
		for kk := 0; kk < k; kk += blockSizeF32 {
			kEnd := min(kk+blockSizeF32, k)
			for jj := 0; jj < n; jj += blockSizeF32 {
				jEnd := min(jj+blockSizeF32, n)
				matmulMicroKernelF32(c, a, b, k, n, ii, iEnd, kk, kEnd, jj, jEnd)
			}
		}
	}
}

// matmulMicroKernelF32 accumulates the block product A[ii:iEnd, kk:kEnd] × B[kk:kEnd, jj:jEnd]
// into C[ii:iEnd, jj:jEnd].
//
// When an AVX2 SIMD kernel is available (amd64, built with GOEXPERIMENT=simd),
// simdMicroKernelF32 is non-nil and the work is delegated to avx2MicroKernelF32.
// Otherwise the scalar i→k→j loop runs: aVal is hoisted out of the j-loop and
// b[kIdx*n+j] access is sequential (row-major) to maximize cache utilization.
func matmulMicroKernelF32(c, a, b []float32, k, n, ii, iEnd, kk, kEnd, jj, jEnd int) {
	if simdMicroKernelF32 != nil {
		simdMicroKernelF32(c, a, b, k, n, ii, iEnd, kk, kEnd, jj, jEnd)
		return
	}
	for i := ii; i < iEnd; i++ {
		for kIdx := kk; kIdx < kEnd; kIdx++ {
			aVal := a[i*k+kIdx]
			for j := jj; j < jEnd; j++ {
				c[i*n+j] += aVal * b[kIdx*n+j]
			}
		}
	}
}

// matmulNaiveFloat32 is the plain O(n³) fallback used for small matrices.
func matmulNaiveFloat32(c, a, b []float32, m, k, n int) {
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			sum := float32(0)
			for kIdx := 0; kIdx < k; kIdx++ {
				sum += a[i*k+kIdx] * b[kIdx*n+j]
			}
			c[i*n+j] = sum
		}
	}
}

// matmulFloat64 multiplies A (m×k) by B (k×n) and writes the result into C (m×n).
//
// Identical algorithm to matmulFloat32 but with blockSizeF64 = 32 so that a
// 32×32 block of float64 still fits within a 32 KB L1 data cache (8 KB used).
//
// Like matmulFloat32, C is OVERWRITTEN (zeroed up front), not accumulated into;
// pointwiseConvFloat64 relies on that to skip its own zeroing.
//
// The caller must pass c pre-capped to exactly m*n elements.
func matmulFloat64(c, a, b []float64, m, k, n int) {
	for i := range c {
		c[i] = 0
	}

	if m*k*n < blockThreshold {
		matmulNaiveFloat64(c, a, b, m, k, n)
		return
	}

	for ii := 0; ii < m; ii += blockSizeF64 {
		iEnd := min(ii+blockSizeF64, m)
		for kk := 0; kk < k; kk += blockSizeF64 {
			kEnd := min(kk+blockSizeF64, k)
			for jj := 0; jj < n; jj += blockSizeF64 {
				jEnd := min(jj+blockSizeF64, n)
				matmulMicroKernelF64(c, a, b, k, n, ii, iEnd, kk, kEnd, jj, jEnd)
			}
		}
	}
}

// matmulMicroKernelF64 accumulates the block product A[ii:iEnd, kk:kEnd] × B[kk:kEnd, jj:jEnd]
// into C[ii:iEnd, jj:jEnd].
func matmulMicroKernelF64(c, a, b []float64, k, n, ii, iEnd, kk, kEnd, jj, jEnd int) {
	for i := ii; i < iEnd; i++ {
		for kIdx := kk; kIdx < kEnd; kIdx++ {
			aVal := a[i*k+kIdx]
			for j := jj; j < jEnd; j++ {
				c[i*n+j] += aVal * b[kIdx*n+j]
			}
		}
	}
}

// matmulNaiveFloat64 is the plain O(n³) fallback used for small float64 matrices.
func matmulNaiveFloat64(c, a, b []float64, m, k, n int) {
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			sum := float64(0)
			for kIdx := 0; kIdx < k; kIdx++ {
				sum += a[i*k+kIdx] * b[kIdx*n+j]
			}
			c[i*n+j] = sum
		}
	}
}

func matmulInt32(c, a, b []int32, m, k, n int) {
	for i := range c {
		c[i] = 0
	}

	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			sum := int32(0)
			for kIdx := 0; kIdx < k; kIdx++ {
				sum += a[i*k+kIdx] * b[kIdx*n+j]
			}
			c[i*n+j] = sum
		}
	}
}

func matmulInt64(c, a, b []int64, m, k, n int) {
	for i := range c {
		c[i] = 0
	}

	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			sum := int64(0)
			for kIdx := 0; kIdx < k; kIdx++ {
				sum += a[i*k+kIdx] * b[kIdx*n+j]
			}
			c[i*n+j] = sum
		}
	}
}
