package cpu

import (
	"fmt"
	"math/rand"
	"testing"
)

// createRandomInt64Slice returns a slice of length n filled with
// random int64 values, suitable for benchmarking element-wise ops.
func createRandomInt64Slice(n int) []int64 {
	a := make([]int64, n)

	rng := rand.New(rand.NewSource(0))
	for i := range a {
		a[i] = int64(rng.Int())
	}
	return a
}

// BenchmarkAddInplaceI64_Scalar benchmarks a[i] += b[i] using the scalar fallback.
func BenchmarkAddInplaceI64_Scalar(b *testing.B) {
	saved := simdAddInplaceInt64
	simdAddInplaceInt64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				addInplaceInt64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdAddInplaceInt64 = saved
}

// BenchmarkAddInplaceI64_SIMD benchmarks a[i] += b[i] using the SIMD implementation.
func BenchmarkAddInplaceI64_SIMD(b *testing.B) {
	if simdAddInplaceInt64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				addInplaceInt64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSubInplaceI64_Scalar benchmarks a[i] -= b[i] using the scalar fallback.
func BenchmarkSubInplaceI64_Scalar(b *testing.B) {
	saved := simdSubInplaceInt64
	simdSubInplaceInt64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				subInplaceInt64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdSubInplaceInt64 = saved
}

// BenchmarkSubInplaceI64_SIMD benchmarks a[i] -= b[i] using the SIMD implementation.
func BenchmarkSubInplaceI64_SIMD(b *testing.B) {
	if simdSubInplaceInt64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				subInplaceInt64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkMulInplaceI64_Scalar benchmarks a[i] *= b[i] using the scalar fallback.
func BenchmarkMulInplaceI64_Scalar(b *testing.B) {
	saved := simdMulInplaceInt64
	simdMulInplaceInt64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				mulInplaceInt64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdMulInplaceInt64 = saved
}

// BenchmarkMulInplaceI64_SIMD benchmarks a[i] *= b[i] using the SIMD implementation.
func BenchmarkMulInplaceI64_SIMD(b *testing.B) {
	if simdMulInplaceInt64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			b.ResetTimer()
			for b.Loop() {
				mulInplaceInt64(aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkAddVectorizedI64_Scalar benchmarks dst[i] = a[i] + b[i] using the scalar fallback.
func BenchmarkAddVectorizedI64_Scalar(b *testing.B) {
	saved := simdAddVectorizedInt64
	simdAddVectorizedInt64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			dst := make([]int64, size)
			b.ResetTimer()
			for b.Loop() {
				addVectorizedInt64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdAddVectorizedInt64 = saved
}

// BenchmarkAddVectorizedI64_SIMD benchmarks dst[i] = a[i] + b[i] using the SIMD implementation.
func BenchmarkAddVectorizedI64_SIMD(b *testing.B) {
	if simdAddVectorizedInt64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			dst := make([]int64, size)
			b.ResetTimer()
			for b.Loop() {
				addVectorizedInt64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkSubVectorizedI64_Scalar benchmarks dst[i] = a[i] - b[i] using the scalar fallback.
func BenchmarkSubVectorizedI64_Scalar(b *testing.B) {
	saved := simdSubVectorizedInt64
	simdSubVectorizedInt64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			dst := make([]int64, size)
			b.ResetTimer()
			for b.Loop() {
				subVectorizedInt64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdSubVectorizedInt64 = saved
}

// BenchmarkSubVectorizedI64_SIMD benchmarks dst[i] = a[i] - b[i] using the SIMD implementation.
func BenchmarkSubVectorizedI64_SIMD(b *testing.B) {
	if simdSubVectorizedInt64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			dst := make([]int64, size)
			b.ResetTimer()
			for b.Loop() {
				subVectorizedInt64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}

// BenchmarkMulVectorizedI64_Scalar benchmarks dst[i] = a[i] * b[i] using the scalar fallback.
func BenchmarkMulVectorizedI64_Scalar(b *testing.B) {
	saved := simdMulVectorizedInt64
	simdMulVectorizedInt64 = nil
	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			dst := make([]int64, size)
			b.ResetTimer()
			for b.Loop() {
				mulVectorizedInt64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
	simdMulVectorizedInt64 = saved
}

// BenchmarkMulVectorizedI64_SIMD benchmarks dst[i] = a[i] * b[i] using the SIMD implementation.
func BenchmarkMulVectorizedI64_SIMD(b *testing.B) {
	if simdMulVectorizedInt64 == nil {
		b.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, size := range simdBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			aSlice := createRandomInt64Slice(size)
			bSlice := createRandomInt64Slice(size)
			dst := make([]int64, size)
			b.ResetTimer()
			for b.Loop() {
				mulVectorizedInt64(dst, aSlice, bSlice)
			}
			b.SetBytes(int64(size * 8))
		})
	}
}
