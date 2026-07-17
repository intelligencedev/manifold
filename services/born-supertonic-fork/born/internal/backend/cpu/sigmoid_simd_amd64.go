//go:build amd64 && goexperiment.simd

package cpu

import "simd/archsimd"

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdSigmoidFloat32 func(dst, src []float32)
var simdSigmoidFloat64 func(dst, src []float64)

func init() {
	if archsimd.X86.AVX2() {
		if expFloat32x8 != nil {
			simdSigmoidFloat32 = avx2SigmoidFloat32
		}
		if expFloat64x4 != nil {
			simdSigmoidFloat64 = avx2SigmoidFloat64
		}
	}
}

func avx2SigmoidFloat32(dst, src []float32) {
	n := len(src)
	i := 0

	negOnesLoaded := archsimd.BroadcastFloat32x8(float32(-1.0))
	onesLoaded := archsimd.BroadcastFloat32x8(float32(1.0))

	for ; i+8 <= n; i += 8 {
		srcLoaded := archsimd.LoadFloat32x8Slice(src[i:])

		negSrc := srcLoaded.Mul(negOnesLoaded)

		divisor := onesLoaded.Add(expFloat32x8(negSrc))
		result := onesLoaded.Div(divisor)

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	sigmoidScalar(dst[i:], src[i:])
}

func avx2SigmoidFloat64(dst, src []float64) {
	n := len(src)
	i := 0

	negOnesLoaded := archsimd.BroadcastFloat64x4(-1.0)
	onesLoaded := archsimd.BroadcastFloat64x4(1.0)

	for ; i+4 <= n; i += 4 {
		srcLoaded := archsimd.LoadFloat64x4Slice(src[i:])

		negSrc := srcLoaded.Mul(negOnesLoaded)

		divisor := onesLoaded.Add(expFloat64x4(negSrc))
		result := onesLoaded.Div(divisor)

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	sigmoidScalar(dst[i:], src[i:])
}
