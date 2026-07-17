package cpu

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tolerance"
)

// createRandomUint8Slice returns a slice of length n filled with
// random uint8 values in [0, 255], suitable for benchmarking element-wise ops.
func createRandomUint8Slice(n int) []uint8 {
	a := make([]uint8, n)
	rng := rand.New(rand.NewSource(0))
	for i := range a {
		a[i] = uint8(rng.Uint32())
	}
	return a
}

// int32SpecialCases returns a random int32, choosing with equal
// probability between MinInt32, MaxInt32, 0, or a random int32.
func int32SpecialCases(rng *rand.Rand) int32 {
	a := rng.Float32()
	switch {
	case a < 0.25:
		return math.MinInt32
	case a < 0.50:
		return math.MaxInt32
	case a < 0.75:
		return int32(0)
	default:
		isNegative := rng.Intn(2)
		sign := int32(1)
		if isNegative == 1 {
			sign = -1
		}
		return rng.Int31() * sign
	}
}

// int64SpecialCases returns a random int64, choosing with equal
// probability between MinInt64, MaxInt64, 0, or a random int32.
func int64SpecialCases(rng *rand.Rand) int64 {
	a := rng.Float32()
	switch {
	case a < 0.25:
		return math.MinInt64
	case a < 0.50:
		return math.MaxInt64
	case a < 0.75:
		return int64(0)
	default:
		isNegative := rng.Intn(2)
		sign := int64(1)
		if isNegative == 1 {
			sign = -1
		}
		return rng.Int63() * sign
	}
}

// simdSignTestCase is a struct to facilitate table-driven SIMD tests on sign.
type simdSignTestCase[T uint8 | int32 | int64 | float32 | float64] struct {
	name         string
	srcGenerator func(*rand.Rand) T
}

// TestSignF32_SIMDMatchesScalar verifies that the SIMD float32 sign matches the scalar result.
func TestSignF32_SIMDMatchesScalar(t *testing.T) {
	if simdSignFloat32 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := &tolerance.Tolerance[float32]{
		TolType: tolerance.Abs,
		Abs:     0.0,
	}
	rng := rand.New(rand.NewSource(1))

	cases := []simdSignTestCase[float32]{
		{name: "unit", srcGenerator: float32Unit},
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

				signFloats(dstScalar, src)
				simdSignFloat32(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestSignF64_SIMDMatchesScalar verifies that the SIMD float64 sign matches the scalar result.
func TestSignF64_SIMDMatchesScalar(t *testing.T) {
	if simdSignFloat64 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := &tolerance.Tolerance[float64]{
		TolType: tolerance.Abs,
		Abs:     0.0,
	}
	rng := rand.New(rand.NewSource(1))

	cases := []simdSignTestCase[float64]{
		{name: "unit", srcGenerator: float64Unit},
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

				signFloats(dstScalar, src)
				simdSignFloat64(dstSIMD, src)

				if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestSignI32_SIMDMatchesScalar verifies that the SIMD int32 sign matches the scalar result.
func TestSignInt32_SIMDMatchesScalar(t *testing.T) {
	if simdSignInt32 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	rng := rand.New(rand.NewSource(1))

	cases := []simdSignTestCase[int32]{
		{name: "range 300", srcGenerator: int32Range300},
		{name: "special", srcGenerator: int32SpecialCases},
	}

	for _, c := range cases {
		for _, size := range simdTestSliceLengths {
			t.Run(fmt.Sprintf("%s(size=%d)", c.name, size), func(t *testing.T) {
				src := make([]int32, size)
				dstScalar := make([]int32, size)
				dstSIMD := make([]int32, size)

				for i := range src {
					src[i] = c.srcGenerator(rng)
				}

				signInts(dstScalar, src)
				simdSignInt32(dstSIMD, src)

				for i := range src {
					if dstScalar[i] != dstSIMD[i] {
						t.Fatalf("src[%d] = %d, scalar = %d, SIMD = %d", i, src[i], dstScalar[i], dstSIMD[i])
					}
				}
			})
		}
	}
}

// TestSignI64_SIMDMatchesScalar verifies that the SIMD int64 sign matches the scalar result.
func TestSignInt64_SIMDMatchesScalar(t *testing.T) {
	if simdSignInt64 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	rng := rand.New(rand.NewSource(1))

	cases := []simdSignTestCase[int64]{
		{name: "range 300", srcGenerator: int64Range300},
		{name: "special", srcGenerator: int64SpecialCases},
	}

	for _, c := range cases {
		for _, size := range simdTestSliceLengths {
			t.Run(fmt.Sprintf("%s(size=%d)", c.name, size), func(t *testing.T) {
				src := make([]int64, size)
				dstScalar := make([]int64, size)
				dstSIMD := make([]int64, size)

				for i := range src {
					src[i] = c.srcGenerator(rng)
				}

				signInts(dstScalar, src)
				simdSignInt64(dstSIMD, src)

				for i := range src {
					if dstScalar[i] != dstSIMD[i] {
						t.Fatalf("src[%d] = %d, scalar = %d, SIMD = %d", i, src[i], dstScalar[i], dstSIMD[i])
					}
				}
			})
		}
	}
}

// TestSignU8_SIMDMatchesScalar verifies that the SIMD uint8 sign matches the scalar result.
func TestSignU8_SIMDMatchesScalar(t *testing.T) {
	if simdSignUint8 == nil {
		t.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	rng := rand.New(rand.NewSource(1))

	cases := []simdSignTestCase[uint8]{
		{name: "standard", srcGenerator: func(rng *rand.Rand) uint8 { return uint8(rng.Int()) }},
	}

	for _, c := range cases {
		for _, size := range simdTestSliceLengths {
			t.Run(fmt.Sprintf("%s(size=%d)", c.name, size), func(t *testing.T) {
				src := make([]uint8, size)
				dstScalar := make([]uint8, size)
				dstSIMD := make([]uint8, size)

				for i := range src {
					src[i] = c.srcGenerator(rng)
				}

				signUint8(dstScalar, src)
				simdSignUint8(dstSIMD, src)

				for i := range src {
					if dstScalar[i] != dstSIMD[i] {
						t.Fatalf("src[%d] = %d, scalar = %d, SIMD = %d", i, src[i], dstScalar[i], dstSIMD[i])
					}
				}
			})
		}
	}
}

// BenchmarkSignF32_Scalar benchmarks float32 sign using the scalar fallback.
func BenchmarkSignF32_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				signFloats(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSignF32_SIMD benchmarks float32 sign using the SIMD implementation.
func BenchmarkSignF32_SIMD(b *testing.B) {
	if simdSignFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				simdSignFloat32(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSignF64_Scalar benchmarks float64 sign using the scalar fallback.
func BenchmarkSignF64_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				signFloats(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSignF64_SIMD benchmarks float64 sign using the SIMD implementation.
func BenchmarkSignF64_SIMD(b *testing.B) {
	if simdSignFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				simdSignFloat64(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSignI32_Scalar benchmarks int32 sign using the scalar fallback.
func BenchmarkSignI32_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomInt32Slice(size)
			dst := make([]int32, size)
			b.ResetTimer()
			for b.Loop() {
				signInts(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSignI32_SIMD benchmarks int32 sign using the SIMD implementation.
func BenchmarkSignI32_SIMD(b *testing.B) {
	if simdSignInt32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomInt32Slice(size)
			dst := make([]int32, size)
			b.ResetTimer()
			for b.Loop() {
				simdSignInt32(dst, src)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSignI64_Scalar benchmarks int64 sign using the scalar fallback.
func BenchmarkSignI64_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomInt64Slice(size)
			dst := make([]int64, size)
			b.ResetTimer()
			for b.Loop() {
				signInts(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSignI64_SIMD benchmarks int64 sign using the SIMD implementation.
func BenchmarkSignI64_SIMD(b *testing.B) {
	if simdSignInt64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomInt64Slice(size)
			dst := make([]int64, size)
			b.ResetTimer()
			for b.Loop() {
				simdSignInt64(dst, src)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSignU8_Scalar benchmarks uint8 sign using the scalar fallback.
func BenchmarkSignU8_Scalar(b *testing.B) {
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomUint8Slice(size)
			dst := make([]uint8, size)
			b.ResetTimer()
			for b.Loop() {
				signUint8(dst, src)
			}
			b.SetBytes(int64(size))
		})
	}
}

// BenchmarkSignU8_SIMD benchmarks uint8 sign using the SIMD implementation.
func BenchmarkSignU8_SIMD(b *testing.B) {
	if simdSignUint8 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			src := createRandomUint8Slice(size)
			dst := make([]uint8, size)
			b.ResetTimer()
			for b.Loop() {
				simdSignUint8(dst, src)
			}
			b.SetBytes(int64(size))
		})
	}
}
