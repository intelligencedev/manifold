package cpu

import (
	"fmt"
	"math/rand"
	"testing"
)

// createRandomInt32Slice returns a slice of length n filled with
// random int32 values, suitable for benchmarking element-wise ops.
func createRandomInt32Slice(n int) []int32 {
	a := make([]int32, n)

	rng := rand.New(rand.NewSource(0))
	for i := range a {
		a[i] = int32(rng.Int())
	}
	return a
}

// BenchmarkAddInplaceI32_Scalar benchmarks a[i] += b[i] using the scalar fallback.
func BenchmarkAddInplaceI32_Scalar(b *testing.B) {
	saved := simdAddInplaceInt32
	simdAddInplaceInt32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				addInplaceInt32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdAddInplaceInt32 = saved
}

// BenchmarkAddInplaceI32_SIMD benchmarks a[i] += b[i] using the SIMD implementation.
func BenchmarkAddInplaceI32_SIMD(b *testing.B) {
	if simdAddInplaceInt32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				addInplaceInt32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSubInplaceI32_Scalar benchmarks a[i] -= b[i] using the scalar fallback.
func BenchmarkSubInplaceI32_Scalar(b *testing.B) {
	saved := simdSubInplaceInt32
	simdSubInplaceInt32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				subInplaceInt32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdSubInplaceInt32 = saved
}

// BenchmarkSubInplaceI32_SIMD benchmarks a[i] -= b[i] using the SIMD implementation.
func BenchmarkSubInplaceI32_SIMD(b *testing.B) {
	if simdSubInplaceInt32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				subInplaceInt32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkMulInplaceI32_Scalar benchmarks a[i] *= b[i] using the scalar fallback.
func BenchmarkMulInplaceI32_Scalar(b *testing.B) {
	saved := simdMulInplaceInt32
	simdMulInplaceInt32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				mulInplaceInt32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdMulInplaceInt32 = saved
}

// BenchmarkMulInplaceI32_SIMD benchmarks a[i] *= b[i] using the SIMD implementation.
func BenchmarkMulInplaceI32_SIMD(b *testing.B) {
	if simdMulInplaceInt32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			b.ResetTimer()
			for b.Loop() {
				mulInplaceInt32(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkAddVectorizedI32_Scalar benchmarks dst[i] = a[i] + b[i] using the scalar fallback.
func BenchmarkAddVectorizedI32_Scalar(b *testing.B) {
	saved := simdAddVectorizedInt32
	simdAddVectorizedInt32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			dst := make([]int32, size)
			b.ResetTimer()
			for b.Loop() {
				addVectorizedInt32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdAddVectorizedInt32 = saved
}

// BenchmarkAddVectorizedI32_SIMD benchmarks dst[i] = a[i] + b[i] using the SIMD implementation.
func BenchmarkAddVectorizedI32_SIMD(b *testing.B) {
	if simdAddVectorizedInt32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			dst := make([]int32, size)
			b.ResetTimer()
			for b.Loop() {
				addVectorizedInt32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkSubVectorizedI32_Scalar benchmarks dst[i] = a[i] - b[i] using the scalar fallback.
func BenchmarkSubVectorizedI32_Scalar(b *testing.B) {
	saved := simdSubVectorizedInt32
	simdSubVectorizedInt32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			dst := make([]int32, size)
			b.ResetTimer()
			for b.Loop() {
				subVectorizedInt32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdSubVectorizedInt32 = saved
}

// BenchmarkSubVectorizedI32_SIMD benchmarks dst[i] = a[i] - b[i] using the SIMD implementation.
func BenchmarkSubVectorizedI32_SIMD(b *testing.B) {
	if simdSubVectorizedInt32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			dst := make([]int32, size)
			b.ResetTimer()
			for b.Loop() {
				subVectorizedInt32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}

// BenchmarkMulVectorizedI32_Scalar benchmarks dst[i] = a[i] * b[i] using the scalar fallback.
func BenchmarkMulVectorizedI32_Scalar(b *testing.B) {
	saved := simdMulVectorizedInt32
	simdMulVectorizedInt32 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			dst := make([]int32, size)
			b.ResetTimer()
			for b.Loop() {
				mulVectorizedInt32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
	simdMulVectorizedInt32 = saved
}

// BenchmarkMulVectorizedI32_SIMD benchmarks dst[i] = a[i] * b[i] using the SIMD implementation.
func BenchmarkMulVectorizedI32_SIMD(b *testing.B) {
	if simdMulVectorizedInt32 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt32Slice(size)
			bSlice := createRandomInt32Slice(size)
			dst := make([]int32, size)
			b.ResetTimer()
			for b.Loop() {
				mulVectorizedInt32(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 4))
		})
	}
}
