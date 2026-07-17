package cpu

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tolerance"
)

type sigmoidTestCase[T float32 | float64] struct {
	name         string
	srcGenerator func(*rand.Rand) T
}

// TestSigmoidF32_SIMDMatchesScalar verifies that the SIMD float32 Sigmoid matches the scalar result.
func TestSigmoidF32_SIMDMatchesScalar(t *testing.T) {
	if simdSigmoidFloat32 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float32]()
	rng := rand.New(rand.NewSource(1))

	cases := []sigmoidTestCase[float32]{
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

				sigmoidScalar(dstScalar, src)
				simdSigmoidFloat32(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestSigmoidF64_SIMDMatchesScalar verifies that the SIMD float64 Sigmoid matches the scalar result.
func TestSigmoidF64_SIMDMatchesScalar(t *testing.T) {
	if simdSigmoidFloat64 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float64]()
	rng := rand.New(rand.NewSource(1))

	cases := []sigmoidTestCase[float64]{
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

				sigmoidScalar(dstScalar, src)
				simdSigmoidFloat64(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// BenchmarkSigmoidF32_Scalar benchmarks float32 Sigmoid using the scalar fallback.
func BenchmarkSigmoidF32_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				sigmoidScalar(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSigmoidF32_SIMD benchmarks float32 Sigmoid using the SIMD implementation.
func BenchmarkSigmoidF32_SIMD(b *testing.B) {
	if simdSigmoidFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				simdSigmoidFloat32(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSigmoidF64_Scalar benchmarks float64 Sigmoid using the scalar fallback.
func BenchmarkSigmoidF64_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				sigmoidScalar(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSigmoidF64_SIMD benchmarks float64 Sigmoid using the SIMD implementation.
func BenchmarkSigmoidF64_SIMD(b *testing.B) {
	if simdSigmoidFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				simdSigmoidFloat64(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}
