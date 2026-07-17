package cpu

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tolerance"
)

type expTestCase[T float32 | float64] struct {
	name         string
	srcGenerator func(*rand.Rand) T
}

// expFloat32SpecialCases returns a random value from expF32SeedOptions.
// These values stress the SIMD range-reduction + polynomial approximation.
func expFloat32SpecialCases(rng *rand.Rand) float32 {
	i := rng.Int()
	opts := expF32SeedOptions()
	return opts[i%len(opts)]
}

// TestExpF32_SIMDMatchesScalar verifies that the SIMD float32 Exp matches the scalar result.
func TestExpF32_SIMDMatchesScalar(t *testing.T) {
	if simdExpFloat32 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float32]()
	rng := rand.New(rand.NewSource(1))

	cases := []expTestCase[float32]{
		{name: "unit", srcGenerator: float32Unit},
		{name: "small", srcGenerator: float32Small},
		{name: "large", srcGenerator: float32Large},
		{name: "float special", srcGenerator: floatSpecialCases[float32]},
		{name: "exp special", srcGenerator: expFloat32SpecialCases},
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

				expScalar(dstScalar, src)
				simdExpFloat32(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// expFloat64SpecialCases returns a random value from expF64SeedOptions.
// These values stress the SIMD range-reduction + polynomial approximation.
func expFloat64SpecialCases(rng *rand.Rand) float64 {
	i := rng.Int()
	opts := expF64SeedOptions()
	return opts[i%len(opts)]
}

// TestExpF64_SIMDMatchesScalar verifies that the SIMD float64 Exp matches the scalar result.
func TestExpF64_SIMDMatchesScalar(t *testing.T) {
	if simdExpFloat64 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float64]()
	rng := rand.New(rand.NewSource(1))

	cases := []expTestCase[float64]{
		{name: "unit", srcGenerator: float64Unit},
		{name: "small", srcGenerator: float64Small},
		{name: "large", srcGenerator: float64Large},
		{name: "float special", srcGenerator: floatSpecialCases[float64]},
		{name: "exp special", srcGenerator: expFloat64SpecialCases},
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

				expScalar(dstScalar, src)
				simdExpFloat64(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// BenchmarkExpF32_Scalar benchmarks float32 Exp using the scalar fallback.
func BenchmarkExpF32_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				expScalar(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkExpF32_SIMD benchmarks float32 Exp using the SIMD implementation.
func BenchmarkExpF32_SIMD(b *testing.B) {
	if simdExpFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				simdExpFloat32(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkExpF64_Scalar benchmarks float64 Exp using the scalar fallback.
func BenchmarkExpF64_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				expScalar(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkExpF64_SIMD benchmarks float32 Exp using the SIMD implementation.
func BenchmarkExpF64_SIMD(b *testing.B) {
	if simdExpFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				simdExpFloat64(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}
