package cpu

import (
	"fmt"
	"math/rand"
	"testing"
)

// createRandomFloat32Slice returns a slice of length n filled with
// random float32 values in [-1, 1), suitable for benchmarking element-wise ops.
func createRandomFloat32Slice(n int) []float32 {
	a := make([]float32, n)
	rng := rand.New(rand.NewSource(0))
	for i := range a {
		a[i] = rng.Float32()*2 - 1
	}
	return a
}

// BenchmarkAddInplaceF32_Scalar benchmarks a[i] += b[i] using the scalar fallback.
func BenchmarkAddInplaceF32_Scalar(b *testing.B) {
	saved := simdAddInplaceFloat32
	simdAddInplaceFloat32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				addInplaceFloat32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdAddInplaceFloat32 = saved
}

// BenchmarkAddInplaceF32_SIMD benchmarks a[i] += b[i] using the SIMD implementation.
func BenchmarkAddInplaceF32_SIMD(b *testing.B) {
	if simdAddInplaceFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				addInplaceFloat32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSubInplaceF32_Scalar benchmarks a[i] -= b[i] using the scalar fallback.
func BenchmarkSubInplaceF32_Scalar(b *testing.B) {
	saved := simdSubInplaceFloat32
	simdSubInplaceFloat32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				subInplaceFloat32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdSubInplaceFloat32 = saved
}

// BenchmarkSubInplaceF32_SIMD benchmarks a[i] -= b[i] using the SIMD implementation.
func BenchmarkSubInplaceF32_SIMD(b *testing.B) {
	if simdSubInplaceFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				subInplaceFloat32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkMulInplaceF32_Scalar benchmarks a[i] *= b[i] using the scalar fallback.
func BenchmarkMulInplaceF32_Scalar(b *testing.B) {
	saved := simdMulInplaceFloat32
	simdMulInplaceFloat32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				mulInplaceFloat32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdMulInplaceFloat32 = saved
}

// BenchmarkMulInplaceF32_SIMD benchmarks a[i] *= b[i] using the SIMD implementation.
func BenchmarkMulInplaceF32_SIMD(b *testing.B) {
	if simdMulInplaceFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				mulInplaceFloat32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkDivInplaceF32_Scalar benchmarks a[i] /= b[i] using the scalar fallback.
func BenchmarkDivInplaceF32_Scalar(b *testing.B) {
	saved := simdDivInplaceFloat32
	simdDivInplaceFloat32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				divInplaceFloat32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdDivInplaceFloat32 = saved
}

// BenchmarkDivInplaceF32_SIMD benchmarks a[i] /= b[i] using the SIMD implementation.
func BenchmarkDivInplaceF32_SIMD(b *testing.B) {
	if simdDivInplaceFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				divInplaceFloat32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkAddVectorizedF32_Scalar benchmarks dst[i] = a[i] + b[i] using the scalar fallback.
func BenchmarkAddVectorizedF32_Scalar(b *testing.B) {
	saved := simdAddVectorizedFloat32
	simdAddVectorizedFloat32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				addVectorizedFloat32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdAddVectorizedFloat32 = saved
}

// BenchmarkAddVectorizedF32_SIMD benchmarks dst[i] = a[i] + b[i] using the SIMD implementation.
func BenchmarkAddVectorizedF32_SIMD(b *testing.B) {
	if simdAddVectorizedFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				addVectorizedFloat32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSubVectorizedF32_Scalar benchmarks dst[i] = a[i] - b[i] using the scalar fallback.
func BenchmarkSubVectorizedF32_Scalar(b *testing.B) {
	saved := simdSubVectorizedFloat32
	simdSubVectorizedFloat32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				subVectorizedFloat32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdSubVectorizedFloat32 = saved
}

// BenchmarkSubVectorizedF32_SIMD benchmarks dst[i] = a[i] - b[i] using the SIMD implementation.
func BenchmarkSubVectorizedF32_SIMD(b *testing.B) {
	if simdSubVectorizedFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				subVectorizedFloat32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkMulVectorizedF32_Scalar benchmarks dst[i] = a[i] * b[i] using the scalar fallback.
func BenchmarkMulVectorizedF32_Scalar(b *testing.B) {
	saved := simdMulVectorizedFloat32
	simdMulVectorizedFloat32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				mulVectorizedFloat32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdMulVectorizedFloat32 = saved
}

// BenchmarkMulVectorizedF32_SIMD benchmarks dst[i] = a[i] * b[i] using the SIMD implementation.
func BenchmarkMulVectorizedF32_SIMD(b *testing.B) {
	if simdMulVectorizedFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				mulVectorizedFloat32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkDivVectorizedF32_Scalar benchmarks dst[i] = a[i] / b[i] using the scalar fallback.
func BenchmarkDivVectorizedF32_Scalar(b *testing.B) {
	saved := simdDivVectorizedFloat32
	simdDivVectorizedFloat32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				divVectorizedFloat32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdDivVectorizedFloat32 = saved
}

// BenchmarkDivVectorizedF32_SIMD benchmarks dst[i] = a[i] / b[i] using the SIMD implementation.
func BenchmarkDivVectorizedF32_SIMD(b *testing.B) {
	if simdDivVectorizedFloat32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomFloat32Slice(size)
			bSlice := createRandomFloat32Slice(size)
			dst := make([]float32, size)
			b.ResetTimer()
			for b.Loop() {
				divVectorizedFloat32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}
