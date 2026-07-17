package cpu

import (
	"fmt"
	"math/rand"
	"testing"
)

// createRandomFloat64Slice returns a slice of length n filled with
// random float64 values in [-1, 1), suitable for benchmarking element-wise ops.
func createRandomFloat64Slice(n int) []float64 {
	a := make([]float64, n)
	rng := rand.New(rand.NewSource(0))
	for i := range a {
		a[i] = rng.Float64()*2 - 1
	}
	return a
}

// BenchmarkAddInplaceF64_Scalar benchmarks a[i] += b[i] using the scalar fallback.
func BenchmarkAddInplaceF64_Scalar(b *testing.B) {
	saved := simdAddInplaceFloat64
	simdAddInplaceFloat64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				addInplaceFloat64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdAddInplaceFloat64 = saved
}

// BenchmarkAddInplaceF64_SIMD benchmarks a[i] += b[i] using the SIMD implementation.
func BenchmarkAddInplaceF64_SIMD(b *testing.B) {
	if simdAddInplaceFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				addInplaceFloat64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSubInplaceF64_Scalar benchmarks a[i] -= b[i] using the scalar fallback.
func BenchmarkSubInplaceF64_Scalar(b *testing.B) {
	saved := simdSubInplaceFloat64
	simdSubInplaceFloat64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				subInplaceFloat64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdSubInplaceFloat64 = saved
}

// BenchmarkSubInplaceF64_SIMD benchmarks a[i] -= b[i] using the SIMD implementation.
func BenchmarkSubInplaceF64_SIMD(b *testing.B) {
	if simdSubInplaceFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				subInplaceFloat64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkMulInplaceF64_Scalar benchmarks a[i] *= b[i] using the scalar fallback.
func BenchmarkMulInplaceF64_Scalar(b *testing.B) {
	saved := simdMulInplaceFloat64
	simdMulInplaceFloat64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				mulInplaceFloat64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdMulInplaceFloat64 = saved
}

// BenchmarkMulInplaceF64_SIMD benchmarks a[i] *= b[i] using the SIMD implementation.
func BenchmarkMulInplaceF64_SIMD(b *testing.B) {
	if simdMulInplaceFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				mulInplaceFloat64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkDivInplaceF64_Scalar benchmarks a[i] /= b[i] using the scalar fallback.
func BenchmarkDivInplaceF64_Scalar(b *testing.B) {
	saved := simdDivInplaceFloat64
	simdDivInplaceFloat64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				divInplaceFloat64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdDivInplaceFloat64 = saved
}

// BenchmarkDivInplaceF64_SIMD benchmarks a[i] /= b[i] using the SIMD implementation.
func BenchmarkDivInplaceF64_SIMD(b *testing.B) {
	if simdDivInplaceFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				divInplaceFloat64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkAddVectorizedF64_Scalar benchmarks dst[i] = a[i] + b[i] using the scalar fallback.
func BenchmarkAddVectorizedF64_Scalar(b *testing.B) {
	saved := simdAddVectorizedFloat64
	simdAddVectorizedFloat64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				addVectorizedFloat64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdAddVectorizedFloat64 = saved
}

// BenchmarkAddVectorizedF64_SIMD benchmarks dst[i] = a[i] + b[i] using the SIMD implementation.
func BenchmarkAddVectorizedF64_SIMD(b *testing.B) {
	if simdAddVectorizedFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				addVectorizedFloat64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSubVectorizedF64_Scalar benchmarks dst[i] = a[i] - b[i] using the scalar fallback.
func BenchmarkSubVectorizedF64_Scalar(b *testing.B) {
	saved := simdSubVectorizedFloat64
	simdSubVectorizedFloat64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				subVectorizedFloat64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdSubVectorizedFloat64 = saved
}

// BenchmarkSubVectorizedF64_SIMD benchmarks dst[i] = a[i] - b[i] using the SIMD implementation.
func BenchmarkSubVectorizedF64_SIMD(b *testing.B) {
	if simdSubVectorizedFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				subVectorizedFloat64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkMulVectorizedF64_Scalar benchmarks dst[i] = a[i] * b[i] using the scalar fallback.
func BenchmarkMulVectorizedF64_Scalar(b *testing.B) {
	saved := simdMulVectorizedFloat64
	simdMulVectorizedFloat64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				mulVectorizedFloat64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdMulVectorizedFloat64 = saved
}

// BenchmarkMulVectorizedF64_SIMD benchmarks dst[i] = a[i] * b[i] using the SIMD implementation.
func BenchmarkMulVectorizedF64_SIMD(b *testing.B) {
	if simdMulVectorizedFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				mulVectorizedFloat64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkDivVectorizedF64_Scalar benchmarks dst[i] = a[i] / b[i] using the scalar fallback.
func BenchmarkDivVectorizedF64_Scalar(b *testing.B) {
	saved := simdDivVectorizedFloat64
	simdDivVectorizedFloat64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				divVectorizedFloat64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdDivVectorizedFloat64 = saved
}

// BenchmarkDivVectorizedF64_SIMD benchmarks dst[i] = a[i] / b[i] using the SIMD implementation.
func BenchmarkDivVectorizedF64_SIMD(b *testing.B) {
	if simdDivVectorizedFloat64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat64Slice(size)
			bSlice := createRandomFloat64Slice(size)
			dst := make([]float64, size)
			b.ResetTimer()
			for b.Loop() {
				divVectorizedFloat64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}
