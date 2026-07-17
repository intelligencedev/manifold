//go:build amd64 && goexperiment.simd

package cpu

import "simd/archsimd"

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdReluFloat32 func(dst, src []float32)
var simdReluFloat64 func(dst, src []float64)

func init() {
	if archsimd.X86.AVX() {
		simdReluFloat32 = avxReluFloat32
		simdReluFloat64 = avxReluFloat64
	}
	if archsimd.X86.AVX512() {
		simdReluFloat32 = avx512ReluFloat32
		simdReluFloat64 = avx512ReluFloat64
	}
}

// avxReluFloat32 computes dst[i] = max(0, src[i]) using AVX (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avxReluFloat32(dst, src []float32) {
	n := len(src)
	i := 0

	zerosLoaded := archsimd.BroadcastFloat32x8(float32(0.0))

	for ; i+8 <= n; i += 8 {
		srcLoaded := archsimd.LoadFloat32x8Slice(src[i:])
		mask := srcLoaded.Greater(zerosLoaded)
		result := srcLoaded.Masked(mask)
		result.StoreSlice(dst[i:])
	}

	// scalar tail
	reluScalar(dst[i:], src[i:])
}

// avx512ReluFloat32 computes dst[i] = max(0, src[i]) using AVX (512-bit, 16 float32/vector).
// Processes 16 elements per vector iteration with a scalar tail for the final 0-15 elements.
func avx512ReluFloat32(dst, src []float32) {
	n := len(src)
	i := 0

	zerosLoaded := archsimd.BroadcastFloat32x16(float32(0.0))

	for ; i+16 <= n; i += 16 {
		srcLoaded := archsimd.LoadFloat32x16Slice(src[i:])
		mask := srcLoaded.Greater(zerosLoaded)
		result := srcLoaded.Masked(mask)
		result.StoreSlice(dst[i:])
	}

	// scalar tail
	reluScalar(dst[i:], src[i:])
}

// avxReluFloat64 computes dst[i] = max(0, src[i]) using AVX (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avxReluFloat64(dst, src []float64) {
	n := len(src)
	i := 0

	zerosLoaded := archsimd.BroadcastFloat64x4(float64(0.0))

	for ; i+4 <= n; i += 4 {
		srcLoaded := archsimd.LoadFloat64x4Slice(src[i:])
		mask := srcLoaded.Greater(zerosLoaded)
		result := srcLoaded.Masked(mask)
		result.StoreSlice(dst[i:])
	}

	// scalar tail
	reluScalar(dst[i:], src[i:])
}

// avx512ReluFloat64 computes dst[i] = max(0, src[i]) using AVX (512-bit, 8 float64/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx512ReluFloat64(dst, src []float64) {
	n := len(src)
	i := 0

	zerosLoaded := archsimd.BroadcastFloat64x8(float64(0.0))

	for ; i+8 <= n; i += 8 {
		srcLoaded := archsimd.LoadFloat64x8Slice(src[i:])
		mask := srcLoaded.Greater(zerosLoaded)
		result := srcLoaded.Masked(mask)
		result.StoreSlice(dst[i:])
	}

	// scalar tail
	reluScalar(dst[i:], src[i:])
}
