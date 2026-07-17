package cpu

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tolerance"
)

type siluTestCase[T float32 | float64] struct {
	name         string
	srcGenerator func(*rand.Rand) T
}

// TestSiLUF32_SIMDMatchesScalar verifies that the SIMD float32 SiLU matches the scalar result.
func TestSiLUF32_SIMDMatchesScalar(t *testing.T) {
	if simdSiluFloat32 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float32]()
	rng := rand.New(rand.NewSource(1))

	cases := []siluTestCase[float32]{
		{name: "unit", srcGenerator: float32Unit},
		{name: "small", srcGenerator: float32Small},
		{name: "large", srcGenerator: float32Large},
		{name: "special", srcGenerator: floatSpecialCases[float32]},
	}

	for _, c := range cases {
		for _, size := range simdTestSliceLengths {
			t.Run(fmt.Sprintf("%s(size=%d)", c.name, size), func(t *testing.T) {
				src := make([]float32, size)
				dstScalar := make([]float32, size)
				dstSIMD := make([]float32, size)

				for i := range src {
					src[i] = c.srcGenerator(rng)
				}

				siluScalar(dstScalar, src)
				simdSiluFloat32(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestSiLUF64_SIMDMatchesScalar verifies that the SIMD float64 SiLU matches the scalar result.
func TestSiLUF64_SIMDMatchesScalar(t *testing.T) {
	if simdSiluFloat64 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float64]()
	rng := rand.New(rand.NewSource(1))

	cases := []siluTestCase[float64]{
		{name: "unit", srcGenerator: float64Unit},
		{name: "small", srcGenerator: float64Small},
		{name: "large", srcGenerator: float64Large},
		{name: "special", srcGenerator: floatSpecialCases[float64]},
	}

	for _, c := range cases {
		for _, size := range simdTestSliceLengths {
			t.Run(fmt.Sprintf("%s(size=%d)", c.name, size), func(t *testing.T) {
				src := make([]float64, size)
				dstScalar := make([]float64, size)
				dstSIMD := make([]float64, size)

				for i := range src {
					src[i] = c.srcGenerator(rng)
				}

				siluScalar(dstScalar, src)
				simdSiluFloat64(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// BenchmarkSiluF32_Scalar benchmarks float32 Silu using the scalar fallback.
func BenchmarkSiluF32_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				siluScalar(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSiluF32_SIMD benchmarks float32 Silu using the SIMD implementation.
func BenchmarkSiluF32_SIMD(b *testing.B) {
	if simdSiluFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				simdSiluFloat32(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSiluF64_Scalar benchmarks float64 Silu using the scalar fallback.
func BenchmarkSiluF64_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				siluScalar(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSiluF64_SIMD benchmarks float64 Silu using the SIMD implementation.
func BenchmarkSiluF64_SIMD(b *testing.B) {
	if simdSiluFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				simdSiluFloat64(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}
