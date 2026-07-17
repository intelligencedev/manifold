//go:build amd64 && goexperiment.simd

package cpu

import "simd/archsimd"

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdSiluFloat32 func(dst, src []float32)
var simdSiluFloat64 func(dst, src []float64)

func init() {
	if archsimd.X86.AVX2() {
		if expFloat32x8 != nil {
			simdSiluFloat32 = avxSiluFloat32
		}
		if expFloat64x4 != nil {
			simdSiluFloat64 = avxSiluFloat64
		}
	}
}

func avxSiluFloat32(dst, src []float32) {
	n := len(src)
	i := 0

	negOnesLoaded := archsimd.BroadcastFloat32x8(float32(-1.0))
	onesLoaded := archsimd.BroadcastFloat32x8(float32(1.0))

	for ; i+8 <= n; i += 8 {
		srcLoaded := archsimd.LoadFloat32x8Slice(src[i:])

		negSrc := srcLoaded.Mul(negOnesLoaded)

		divisor := onesLoaded.Add(expFloat32x8(negSrc))
		sigmoid := onesLoaded.Div(divisor)
		result := srcLoaded.Mul(sigmoid)

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	siluScalar(dst[i:], src[i:])
}

func avxSiluFloat64(dst, src []float64) {
	n := len(src)
	i := 0

	negOnesLoaded := archsimd.BroadcastFloat64x4(-1.0)
	onesLoaded := archsimd.BroadcastFloat64x4(1.0)

	for ; i+4 <= n; i += 4 {
		srcLoaded := archsimd.LoadFloat64x4Slice(src[i:])

		negSrc := srcLoaded.Mul(negOnesLoaded)

		divisor := onesLoaded.Add(expFloat64x4(negSrc))
		sigmoid := onesLoaded.Div(divisor)
		result := srcLoaded.Mul(sigmoid)

		result.StoreSlice(dst[i:])
	}

	// scalar tail
	siluScalar(dst[i:], src[i:])
}
