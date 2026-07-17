//go:build amd64 && goexperiment.simd

package cpu

import (
	"simd/archsimd"
)

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdSignFloat32 func(dst, src []float32)
var simdSignFloat64 func(dst, src []float64)
var simdSignInt32 func(dst, src []int32)
var simdSignInt64 func(dst, src []int64)
var simdSignUint8 func(dst, src []uint8)

func init() {
	if archsimd.X86.AVX() {
		simdSignFloat32 = avxSignFloat32
		simdSignFloat64 = avxSignFloat64
	}
	if archsimd.X86.AVX2() {
		simdSignInt32 = avx2SignInt32
		simdSignInt64 = avx2SignInt64
		simdSignUint8 = avx2SignUint8
	}
	if archsimd.X86.AVX512() {
		simdSignFloat32 = avx512SignFloat32
		simdSignFloat64 = avx512SignFloat64
		simdSignInt32 = avx512SignInt32
		simdSignInt64 = avx512SignInt64
		simdSignUint8 = avx512SignUint8
	}
}

// avxSignFloat32 computes dst[i] = sign(src[i]) using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxSignFloat32(dst, src []float32) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastFloat32x8(float32(1.0))
	zerosLoaded := archsimd.BroadcastFloat32x8(float32(0.0))

	for ; i+8 <= n; i += 8 {
		srcLoaded := archsimd.LoadFloat32x8Slice(src[i:])

		positiveMask := srcLoaded.Greater(zerosLoaded)
		negativeMask := srcLoaded.Less(zerosLoaded)
		nanMask := srcLoaded.IsNaN()

		negs := onesLoaded.Masked(negativeMask) // 1.0 where src[i] < 0

		result := onesLoaded.Masked(positiveMask) // 1.0 where src[i] > 0
		result = result.Sub(negs)                 // -1.0 where src[i] < 0, 1.0 where src[i] > 0
		result = srcLoaded.Merge(result, nanMask) // merge in NaNs from src

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	signFloats(dst[i:], src[i:])
}

// avx512SignFloat32 computes dst[i] = sign(src[i]) using AVX512 (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512SignFloat32(dst, src []float32) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastFloat32x16(float32(1.0))
	zerosLoaded := archsimd.BroadcastFloat32x16(float32(0.0))

	for ; i+16 <= n; i += 16 {
		srcLoaded := archsimd.LoadFloat32x16Slice(src[i:])

		positiveMask := srcLoaded.Greater(zerosLoaded)
		negativeMask := srcLoaded.Less(zerosLoaded)
		nanMask := srcLoaded.IsNaN()

		negs := onesLoaded.Masked(negativeMask) // 1.0 where src[i] < 0

		result := onesLoaded.Masked(positiveMask) // 1.0 where src[i] > 0
		result = result.Sub(negs)                 // -1.0 where src[i] < 0, 1.0 where src[i] > 0
		result = srcLoaded.Merge(result, nanMask) // merge in NaNs from src

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	signFloats(dst[i:], src[i:])
}

// avxSignFloat64 computes dst[i] = sign(src[i]) using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxSignFloat64(dst, src []float64) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastFloat64x4(1.0)
	zerosLoaded := archsimd.BroadcastFloat64x4(0.0)

	for ; i+4 <= n; i += 4 {
		srcLoaded := archsimd.LoadFloat64x4Slice(src[i:])

		positiveMask := srcLoaded.Greater(zerosLoaded)
		negativeMask := srcLoaded.Less(zerosLoaded)
		nanMask := srcLoaded.IsNaN()

		negs := onesLoaded.Masked(negativeMask) // 1.0 where src[i] < 0

		result := onesLoaded.Masked(positiveMask) // 1.0 where src[i] > 0
		result = result.Sub(negs)                 // -1.0 where src[i] < 0, 1.0 where src[i] > 0
		result = srcLoaded.Merge(result, nanMask) // merge in NaNs from src

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	signFloats(dst[i:], src[i:])
}

// avx512SignFloat64 computes dst[i] = sign(src[i]) using AVX-512 (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512SignFloat64(dst, src []float64) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastFloat64x8(1.0)
	zerosLoaded := archsimd.BroadcastFloat64x8(0.0)

	for ; i+8 <= n; i += 8 {
		srcLoaded := archsimd.LoadFloat64x8Slice(src[i:])

		positiveMask := srcLoaded.Greater(zerosLoaded)
		negativeMask := srcLoaded.Less(zerosLoaded)
		nanMask := srcLoaded.IsNaN()

		negs := onesLoaded.Masked(negativeMask) // 1.0 where src[i] < 0

		result := onesLoaded.Masked(positiveMask) // 1.0 where src[i] > 0
		result = result.Sub(negs)                 // -1.0 where src[i] < 0, 1.0 where src[i] > 0
		result = srcLoaded.Merge(result, nanMask) // merge in NaNs from src

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	signFloats(dst[i:], src[i:])
}

// avx2SignInt32 computes dst[i] = sign(src[i]) using AVX2 (256-bit, 8 int32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx2SignInt32(dst, src []int32) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastInt32x8(int32(1))
	zerosLoaded := archsimd.BroadcastInt32x8(int32(0))

	for ; i+8 <= n; i += 8 {
		srcLoaded := archsimd.LoadInt32x8Slice(src[i:])

		positiveMask := srcLoaded.Greater(zerosLoaded)
		negativeMask := srcLoaded.Less(zerosLoaded)

		negs := onesLoaded.Masked(negativeMask) // 1 where src[i] < 0

		result := onesLoaded.Masked(positiveMask) // 1 where src[i] > 0
		result = result.Sub(negs)                 // -1 where src[i] < 0, 1 where src[i] > 0

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	signInts(dst[i:], src[i:])
}

// avx512SignInt32 computes dst[i] = sign(src[i]) using AVX-512 (512-bit, 16 int32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512SignInt32(dst, src []int32) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastInt32x16(int32(1))
	zerosLoaded := archsimd.BroadcastInt32x16(int32(0))

	for ; i+16 <= n; i += 16 {
		srcLoaded := archsimd.LoadInt32x16Slice(src[i:])

		positiveMask := srcLoaded.Greater(zerosLoaded)
		negativeMask := srcLoaded.Less(zerosLoaded)

		negs := onesLoaded.Masked(negativeMask) // 1 where src[i] < 0

		result := onesLoaded.Masked(positiveMask) // 1 where src[i] > 0
		result = result.Sub(negs)                 // -1 where src[i] < 0, 1 where src[i] > 0

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	signInts(dst[i:], src[i:])
}

// avx2SignInt64 computes dst[i] = sign(src[i]) using AVX2 (256-bit, 4 int64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avx2SignInt64(dst, src []int64) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastInt64x4(int64(1))
	zerosLoaded := archsimd.BroadcastInt64x4(int64(0))

	for ; i+4 <= n; i += 4 {
		srcLoaded := archsimd.LoadInt64x4Slice(src[i:])

		positiveMask := srcLoaded.Greater(zerosLoaded)
		negativeMask := srcLoaded.Less(zerosLoaded)

		negs := onesLoaded.Masked(negativeMask) // 1 where src[i] < 0

		result := onesLoaded.Masked(positiveMask) // 1 where src[i] > 0
		result = result.Sub(negs)                 // -1 where src[i] < 0, 1 where src[i] > 0

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	signInts(dst[i:], src[i:])
}

// avx512SignInt64 computes dst[i] = sign(src[i]) using AVX-512 (512-bit, 8 int64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512SignInt64(dst, src []int64) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastInt64x8(int64(1))
	zerosLoaded := archsimd.BroadcastInt64x8(int64(0))

	for ; i+8 <= n; i += 8 {
		srcLoaded := archsimd.LoadInt64x8Slice(src[i:])

		positiveMask := srcLoaded.Greater(zerosLoaded)
		negativeMask := srcLoaded.Less(zerosLoaded)

		negs := onesLoaded.Masked(negativeMask) // 1 where src[i] < 0

		result := onesLoaded.Masked(positiveMask) // 1 where src[i] > 0
		result = result.Sub(negs)                 // -1 where src[i] < 0, 1 where src[i] > 0

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	signInts(dst[i:], src[i:])
}

// avx2SignUint8 computes dst[i] = sign(src[i]) using AVX2 (256-bit, 32 uint8/vector).
// Processes 32 elements per vector iteration with a scalar tail for the final 0-31 elements.
func avx2SignUint8(dst, src []uint8) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastUint8x32(uint8(1))
	zerosLoaded := archsimd.BroadcastUint8x32(uint8(0))
	for ; i+32 <= n; i += 32 {
		srcLoaded := archsimd.LoadUint8x32Slice(src[i:])
		mask := srcLoaded.Equal(zerosLoaded)
		merged := srcLoaded.Merge(onesLoaded, mask)
		merged.StoreSlice(dst[i:])
	}

	// scalar tail
	signUint8(dst[i:], src[i:])
}

// avx512SignUint8 computes dst[i] = sign(src[i]) using AVX-512 (512-bit, 64 uint8/vector).
// Processes 64 elements per vector iteration with a scalar tail for the final 0-63 elements.
func avx512SignUint8(dst, src []uint8) {
	n := len(src)
	i := 0

	onesLoaded := archsimd.BroadcastUint8x64(uint8(1))
	zerosLoaded := archsimd.BroadcastUint8x64(uint8(0))
	for ; i+64 <= n; i += 64 {
		srcLoaded := archsimd.LoadUint8x64Slice(src[i:])
		mask := srcLoaded.Equal(zerosLoaded)
		merged := srcLoaded.Merge(onesLoaded, mask)
		merged.StoreSlice(dst[i:])
	}

	// scalar tail
	signUint8(dst[i:], src[i:])
}
