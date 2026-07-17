package cpu

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tolerance"
)

type reluTestCase[T float32 | float64] struct {
	name         string
	srcGenerator func(*rand.Rand) T
}

// TestReluF32_SIMDMatchesScalar verifies that the SIMD float32 Relu matches the scalar result.
func TestReluF32_SIMDMatchesScalar(t *testing.T) {
	if simdReluFloat32 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float32]()
	rng := rand.New(rand.NewSource(1))

	cases := []reluTestCase[float32]{
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

				reluScalar(dstScalar, src)
				simdReluFloat32(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestReluF64_SIMDMatchesScalar verifies that the SIMD float64 Relu matches the scalar result.
func TestReluF64_SIMDMatchesScalar(t *testing.T) {
	if simdReluFloat64 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := &tolerance.Tolerance[float64]{
		TolType: tolerance.Abs,
		Abs:     0.0,
	}
	rng := rand.New(rand.NewSource(1))

	cases := []reluTestCase[float64]{
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

				reluScalar(dstScalar, src)
				simdReluFloat64(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// BenchmarkReluF32_Scalar benchmarks float32 Relu using the scalar fallback.
func BenchmarkReluF32_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				reluScalar(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkReluF32_SIMD benchmarks float32 Relu using the SIMD implementation.
func BenchmarkReluF32_SIMD(b *testing.B) {
	if simdReluFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				simdReluFloat32(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkReluF64_Scalar benchmarks float64 Relu using the scalar fallback.
func BenchmarkReluF64_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				reluScalar(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkReluF64_SIMD benchmarks float64 Relu using the SIMD implementation.
func BenchmarkReluF64_SIMD(b *testing.B) {
	if simdReluFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				simdReluFloat64(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}
