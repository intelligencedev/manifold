package cpu

import (
	"math"
	"math/rand"
	"testing"
)

// TestMatmulMicroKernelF32_SIMDMatchesScalar verifies that avx2MicroKernelF32
// (when available) produces results numerically identical to the scalar
// matmulMicroKernelF32.  When SIMD is not compiled in, simdMicroKernelF32 is
// nil and the test is skipped — the scalar path is exercised by the existing
// TestCPUBackend_MatMul tests.
func TestMatmulMicroKernelF32_SIMDMatchesScalar(t *testing.T) {
	if simdMicroKernelF32 == nil {
		t.Skip("SIMD kernel not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	// maxDiff threshold: AVX2 FMA results must match scalar within float32 ULP noise.
	const maxDiff = 1e-5

	tests := []struct {
		name    string
		m, k, n int
	}{
		{"tiny 1x1", 1, 1, 1},
		{"scalar_tail j not multiple of 8", 4, 8, 13},
		{"8-wide tile exact", 4, 16, 8},
		{"16-wide tile exact", 4, 16, 16},
		{"4-row block exact", 4, 32, 32},
		{"large 128x128", 128, 128, 128},
		{"non-power-of-2 rows", 7, 64, 64},
		{"non-power-of-2 cols", 32, 32, 37},
		{"row fringe 1 row", 1, 64, 64},
		{"row fringe 3 rows", 3, 64, 64},
		{"row fringe 5 rows", 5, 32, 32},
		{"single block fits blockSizeF32", 64, 64, 64},
		{"larger than block", 96, 96, 96},
	}

	rng := rand.New(rand.NewSource(42))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, k, n := tt.m, tt.k, tt.n

			a := make([]float32, m*k)
			b := make([]float32, k*n)
			for i := range a {
				a[i] = rng.Float32()*2 - 1
			}
			for i := range b {
				b[i] = rng.Float32()*2 - 1
			}

			// Scalar reference: process the whole matrix as one block.
			cScalar := make([]float32, m*n)
			// Temporarily nil out the SIMD kernel so the dispatch falls through.
			saved := simdMicroKernelF32
			simdMicroKernelF32 = nil
			matmulMicroKernelF32(cScalar, a, b, k, n, 0, m, 0, k, 0, n)
			simdMicroKernelF32 = saved

			// SIMD result.
			cSIMD := make([]float32, m*n)
			simdMicroKernelF32(cSIMD, a, b, k, n, 0, m, 0, k, 0, n)

			// Compare element-wise.
			for i, got := range cSIMD {
				want := cScalar[i]
				diff := math.Abs(float64(got - want))
				if diff > maxDiff {
					t.Errorf("element[%d]: SIMD=%.8f scalar=%.8f diff=%.2e (limit %.2e)",
						i, got, want, diff, maxDiff)
					if t.Failed() {
						return
					}
				}
			}
		})
	}
}

// BenchmarkMatmulMicroKernelF32_Scalar benchmarks the scalar inner kernel on a
// single 128×128×128 block — useful as a baseline against the SIMD variant.
func BenchmarkMatmulMicroKernelF32_Scalar(b *testing.B) {
	const m, k, n = 128, 128, 128
	a := make([]float32, m*k)
	bMat := make([]float32, k*n)
	c := make([]float32, m*n)
	rng := rand.New(rand.NewSource(0))
	for i := range a {
		a[i] = rng.Float32()
	}
	for i := range bMat {
		bMat[i] = rng.Float32()
	}

	saved := simdMicroKernelF32
	simdMicroKernelF32 = nil
	b.ResetTimer()
	for range b.N {
		for i := range c {
			c[i] = 0
		}
		matmulMicroKernelF32(c, a, bMat, k, n, 0, m, 0, k, 0, n)
	}
	simdMicroKernelF32 = saved
}

// BenchmarkMatmulMicroKernelF32_SIMD benchmarks the AVX2 inner kernel on the
// same 128×128×128 block.  Skips if SIMD is not compiled in.
func BenchmarkMatmulMicroKernelF32_SIMD(b *testing.B) {
	if simdMicroKernelF32 == nil {
		b.Skip("SIMD kernel not available")
	}

	const m, k, n = 128, 128, 128
	a := make([]float32, m*k)
	bMat := make([]float32, k*n)
	c := make([]float32, m*n)
	rng := rand.New(rand.NewSource(0))
	for i := range a {
		a[i] = rng.Float32()
	}
	for i := range bMat {
		bMat[i] = rng.Float32()
	}

	b.ResetTimer()
	for range b.N {
		for i := range c {
			c[i] = 0
		}
		simdMicroKernelF32(c, a, bMat, k, n, 0, m, 0, k, 0, n)
	}
}
