package cpu

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tolerance"
)

// simdBenchmarkSizes is a set of slice lengths to benchmark SIMD operations against.
var simdBenchmarkSizes = []int{1024, 8192, 65536}

// simdSumTestCase is a struct to facilitate table-driven SIMD tests on sum.
type simdSumTestCase[T float32 | float64 | int32 | int64] struct {
	name         string
	srcGenerator func(*rand.Rand) T
}

// floatSpecialCases returns a random float, choosing with equal
// probability between -Inf, +Inf, NaN, or a value in [-1, 1).
func floatSpecialCases[T float32 | float64](rng *rand.Rand) T {
	a := rng.Float32()
	switch {
	case a < 0.25:
		return T(math.Inf(-1))
	case a < 0.50:
		return T(math.Inf(1))
	case a < 0.75:
		return T(math.NaN())
	default:
		return T(rng.Float32()*2 - 1)
	}
}

// TestSumFloat32_ScalarMatchesSIMD verifies that the SIMD float32 sum matches the scalar result.
func TestSumFloat32_ScalarMatchesSIMD(t *testing.T) {
	if simdSumFloat32 == nil {
		t.Skip("SIMD implementation not available")
	}

	cases := []simdSumTestCase[float32]{
		{name: "unit", srcGenerator: float32Unit},
		{name: "small", srcGenerator: float32Small},
		{name: "large", srcGenerator: float32Large},
		{name: "special", srcGenerator: floatSpecialCases[float32]},
	}

	rng := rand.New(rand.NewSource(1))
	tol := tolerance.NewDefaultTolerance[float32]()

	for _, c := range cases {
		for _, n := range simdTestSliceLengths {
			t.Run(fmt.Sprintf("%s(n=%d)", c.name, n), func(t *testing.T) {
				src := make([]float32, n)
				for i := range src {
					src[i] = c.srcGenerator(rng)
				}
				dstSIMD := make([]float32, 1)
				dstScalar := make([]float32, 1)

				sumScalar(dstScalar, src)
				simdSumFloat32(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestSumFloat64_ScalarMatchesSIMD verifies that the SIMD float64 sum matches the scalar result.
func TestSumFloat64_ScalarMatchesSIMD(t *testing.T) {
	if simdSumFloat64 == nil {
		t.Skip("SIMD implementation not available")
	}

	cases := []simdSumTestCase[float64]{
		{name: "unit", srcGenerator: float64Unit},
		{name: "small", srcGenerator: float64Small},
		{name: "large", srcGenerator: float64Large},
		{name: "special", srcGenerator: floatSpecialCases[float64]},
	}

	rng := rand.New(rand.NewSource(1))
	tol := tolerance.NewDefaultTolerance[float64]()

	for _, c := range cases {
		for _, n := range simdTestSliceLengths {
			t.Run(fmt.Sprintf("%s(n=%d)", c.name, n), func(t *testing.T) {
				src := make([]float64, n)
				for i := range src {
					src[i] = c.srcGenerator(rng)
				}
				dstSIMD := make([]float64, 1)
				dstScalar := make([]float64, 1)

				sumScalar(dstScalar, src)
				simdSumFloat64(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestSumInt32_ScalarMatchesSIMD verifies that the SIMD int32 sum matches the scalar result.
func TestSumInt32_ScalarMatchesSIMD(t *testing.T) {
	if simdSumInt32 == nil {
		t.Skip("SIMD implementation not available")
	}

	cases := []simdSumTestCase[int32]{
		{name: "range 300", srcGenerator: int32Range300},
	}

	rng := rand.New(rand.NewSource(1))

	for _, c := range cases {
		for _, n := range simdTestSliceLengths {
			t.Run(fmt.Sprintf("%s(n=%d)", c.name, n), func(t *testing.T) {
				src := make([]int32, n)
				for i := range src {
					src[i] = c.srcGenerator(rng)
				}
				dstSIMD := make([]int32, 1)
				dstScalar := make([]int32, 1)

				sumScalar(dstScalar, src)
				simdSumInt32(dstSIMD, src)

				if dstSIMD[0] != dstScalar[0] {
					t.Errorf("SIMD = %v, scalar = %v, diff = %v", dstSIMD[0], dstScalar[0], dstSIMD[0]-dstScalar[0])
				}
			})
		}
	}
}

// TestSumInt64_ScalarMatchesSIMD verifies that the SIMD int64 sum matches the scalar result.
func TestSumInt64_ScalarMatchesSIMD(t *testing.T) {
	if simdSumInt64 == nil {
		t.Skip("SIMD implementation not available")
	}

	cases := []simdSumTestCase[int64]{
		{name: "range 300", srcGenerator: int64Range300},
	}

	rng := rand.New(rand.NewSource(1))

	for _, c := range cases {
		for _, n := range simdTestSliceLengths {
			t.Run(fmt.Sprintf("%s(n=%d)", c.name, n), func(t *testing.T) {
				src := make([]int64, n)
				for i := range src {
					src[i] = c.srcGenerator(rng)
				}
				dstSIMD := make([]int64, 1)
				dstScalar := make([]int64, 1)

				sumScalar(dstScalar, src)
				simdSumInt64(dstSIMD, src)

				if dstSIMD[0] != dstScalar[0] {
					t.Errorf("SIMD = %v, scalar = %v, diff = %v", dstSIMD[0], dstScalar[0], dstSIMD[0]-dstScalar[0])
				}
			})
		}
	}
}

// BenchmarkSumF32_Scalar benchmarks float32 sum using the scalar fallback.
func BenchmarkSumF32_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, 1)
			b.ResetTimer()
			for b.Loop() {
				sumScalar(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSumF32_SIMD benchmarks float32 sum using the SIMD implementation.
func BenchmarkSumF32_SIMD(b *testing.B) {
	if simdSumFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, 1)
			b.ResetTimer()
			for b.Loop() {
				simdSumFloat32(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSumF64_Scalar benchmarks float64 sum using the scalar fallback.
func BenchmarkSumF64_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, 1)
			b.ResetTimer()
			for b.Loop() {
				sumScalar(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSumF64_SIMD benchmarks float64 sum using the SIMD implementation.
func BenchmarkSumF64_SIMD(b *testing.B) {
	if simdSumFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, 1)
			b.ResetTimer()
			for b.Loop() {
				simdSumFloat64(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSumI32_Scalar benchmarks int32 sum using the scalar fallback.
func BenchmarkSumI32_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomInt32Slice(size)
			dst := make([]int32, 1)
			b.ResetTimer()
			for b.Loop() {
				sumScalar(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSumI32_SIMD benchmarks int32 sum using the SIMD implementation.
func BenchmarkSumI32_SIMD(b *testing.B) {
	if simdSumInt32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomInt32Slice(size)
			dst := make([]int32, 1)
			b.ResetTimer()
			for b.Loop() {
				simdSumInt32(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSumI64_Scalar benchmarks int64 sum using the scalar fallback.
func BenchmarkSumI64_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomInt64Slice(size)
			dst := make([]int64, 1)
			b.ResetTimer()
			for b.Loop() {
				sumScalar(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSumI64_SIMD benchmarks int64 sum using the SIMD implementation.
func BenchmarkSumI64_SIMD(b *testing.B) {
	if simdSumInt64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomInt64Slice(size)
			dst := make([]int64, 1)
			b.ResetTimer()
			for b.Loop() {
				simdSumInt64(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}
