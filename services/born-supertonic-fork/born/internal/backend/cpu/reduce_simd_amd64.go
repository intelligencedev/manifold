//go:build amd64 && goexperiment.simd

package cpu

import (
	"simd/archsimd"
)

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdSumFloat32 func(dst, src []float32)
var simdSumFloat64 func(dst, src []float64)
var simdSumInt32 func(dst, src []int32)
var simdSumInt64 func(dst, src []int64)

func init() {
	if archsimd.X86.AVX() {
		simdSumFloat32 = avxSumFloat32
		simdSumFloat64 = avxSumFloat64
	}
	if archsimd.X86.AVX2() {
		simdSumInt32 = avx2SumInt32
		simdSumInt64 = avx2SumInt64
	}
	if archsimd.X86.AVX512() {
		simdSumFloat32 = avx512SumFloat32
		simdSumFloat64 = avx512SumFloat64
		simdSumInt32 = avx512SumInt32
		simdSumInt64 = avx512SumInt64
	}
}

// avxSumFloat32 computes dst[0] = sum(src[i]) using AVX (256-bit, 8 float32/vector).
// Uses 8 accumulators with a scalar tail for the final 0-63 elements.
func avxSumFloat32(dst, src []float32) {
	n := len(src)
	i := 0
	numAccumulators := 8
	stride := numAccumulators * 8

	acc0, acc1, acc2, acc3 := archsimd.Float32x8{}, archsimd.Float32x8{}, archsimd.Float32x8{}, archsimd.Float32x8{}
	acc4, acc5, acc6, acc7 := archsimd.Float32x8{}, archsimd.Float32x8{}, archsimd.Float32x8{}, archsimd.Float32x8{}

	for ; i+stride <= n; i += stride {
		s := src[i : i+stride]
		acc0 = acc0.Add(archsimd.LoadFloat32x8Slice(s[0:]))
		acc1 = acc1.Add(archsimd.LoadFloat32x8Slice(s[8:]))
		acc2 = acc2.Add(archsimd.LoadFloat32x8Slice(s[16:]))
		acc3 = acc3.Add(archsimd.LoadFloat32x8Slice(s[24:]))
		acc4 = acc4.Add(archsimd.LoadFloat32x8Slice(s[32:]))
		acc5 = acc5.Add(archsimd.LoadFloat32x8Slice(s[40:]))
		acc6 = acc6.Add(archsimd.LoadFloat32x8Slice(s[48:]))
		acc7 = acc7.Add(archsimd.LoadFloat32x8Slice(s[56:]))
	}

	acc01 := acc0.Add(acc1)
	acc23 := acc2.Add(acc3)
	acc45 := acc4.Add(acc5)
	acc67 := acc6.Add(acc7)
	acc0123 := acc01.Add(acc23)
	acc4567 := acc45.Add(acc67)
	acc := acc0123.Add(acc4567)
	accArray := [8]float32{}
	acc.Store(&accArray)

	sum := float32(0.0)
	for _, a := range accArray {
		sum += a
	}
	for ; i < n; i++ {
		sum += src[i]
	}
	dst[0] = sum
}

// avx512SumFloat32 computes dst[0] = sum(src[i]) using AVX-512 (512-bit, 16 float32/vector).
// Uses 8 accumulators with a scalar tail for the final 0-127 elements.
func avx512SumFloat32(dst, src []float32) {
	n := len(src)
	i := 0
	numAccumulators := 8
	stride := numAccumulators * 16

	acc0, acc1, acc2, acc3 := archsimd.Float32x16{}, archsimd.Float32x16{}, archsimd.Float32x16{}, archsimd.Float32x16{}
	acc4, acc5, acc6, acc7 := archsimd.Float32x16{}, archsimd.Float32x16{}, archsimd.Float32x16{}, archsimd.Float32x16{}

	for ; i+stride <= n; i += stride {
		s := src[i : i+stride]
		acc0 = acc0.Add(archsimd.LoadFloat32x16Slice(s[0:]))
		acc1 = acc1.Add(archsimd.LoadFloat32x16Slice(s[16:]))
		acc2 = acc2.Add(archsimd.LoadFloat32x16Slice(s[32:]))
		acc3 = acc3.Add(archsimd.LoadFloat32x16Slice(s[48:]))
		acc4 = acc4.Add(archsimd.LoadFloat32x16Slice(s[64:]))
		acc5 = acc5.Add(archsimd.LoadFloat32x16Slice(s[80:]))
		acc6 = acc6.Add(archsimd.LoadFloat32x16Slice(s[96:]))
		acc7 = acc7.Add(archsimd.LoadFloat32x16Slice(s[112:]))
	}

	acc01 := acc0.Add(acc1)
	acc23 := acc2.Add(acc3)
	acc45 := acc4.Add(acc5)
	acc67 := acc6.Add(acc7)
	acc0123 := acc01.Add(acc23)
	acc4567 := acc45.Add(acc67)
	acc := acc0123.Add(acc4567)

	accArray := [16]float32{}
	acc.Store(&accArray)

	sum := float32(0.0)
	for _, a := range accArray {
		sum += a
	}
	for ; i < n; i++ {
		sum += src[i]
	}
	dst[0] = sum
}

// avxSumFloat64 computes dst[0] = sum(src[i]) using AVX (256-bit, 4 float64/vector).
// Uses 8 accumulators with a scalar tail for the final 0-31 elements.
func avxSumFloat64(dst, src []float64) {
	n := len(src)
	i := 0
	numAccumulators := 8
	stride := numAccumulators * 4

	acc0, acc1, acc2, acc3 := archsimd.Float64x4{}, archsimd.Float64x4{}, archsimd.Float64x4{}, archsimd.Float64x4{}
	acc4, acc5, acc6, acc7 := archsimd.Float64x4{}, archsimd.Float64x4{}, archsimd.Float64x4{}, archsimd.Float64x4{}

	for ; i+stride <= n; i += stride {
		s := src[i : i+stride]
		acc0 = acc0.Add(archsimd.LoadFloat64x4Slice(s[0:]))
		acc1 = acc1.Add(archsimd.LoadFloat64x4Slice(s[4:]))
		acc2 = acc2.Add(archsimd.LoadFloat64x4Slice(s[8:]))
		acc3 = acc3.Add(archsimd.LoadFloat64x4Slice(s[12:]))
		acc4 = acc4.Add(archsimd.LoadFloat64x4Slice(s[16:]))
		acc5 = acc5.Add(archsimd.LoadFloat64x4Slice(s[20:]))
		acc6 = acc6.Add(archsimd.LoadFloat64x4Slice(s[24:]))
		acc7 = acc7.Add(archsimd.LoadFloat64x4Slice(s[28:]))
	}

	acc01 := acc0.Add(acc1)
	acc23 := acc2.Add(acc3)
	acc45 := acc4.Add(acc5)
	acc67 := acc6.Add(acc7)
	acc0123 := acc01.Add(acc23)
	acc4567 := acc45.Add(acc67)
	acc := acc0123.Add(acc4567)

	accArray := [4]float64{}
	acc.Store(&accArray)

	sum := 0.0
	for _, a := range accArray {
		sum += a
	}
	for ; i < n; i++ {
		sum += src[i]
	}
	dst[0] = sum
}

// avx512SumFloat64 computes dst[0] = sum(src[i]) using AVX-512 (512-bit, 8 float64/vector).
// Uses 8 accumulators with a scalar tail for the final 0-63 elements.
func avx512SumFloat64(dst, src []float64) {
	n := len(src)
	i := 0
	numAccumulators := 8
	stride := numAccumulators * 8

	acc0, acc1, acc2, acc3 := archsimd.Float64x8{}, archsimd.Float64x8{}, archsimd.Float64x8{}, archsimd.Float64x8{}
	acc4, acc5, acc6, acc7 := archsimd.Float64x8{}, archsimd.Float64x8{}, archsimd.Float64x8{}, archsimd.Float64x8{}

	for ; i+stride <= n; i += stride {
		s := src[i : i+stride]
		acc0 = acc0.Add(archsimd.LoadFloat64x8Slice(s[0:]))
		acc1 = acc1.Add(archsimd.LoadFloat64x8Slice(s[8:]))
		acc2 = acc2.Add(archsimd.LoadFloat64x8Slice(s[16:]))
		acc3 = acc3.Add(archsimd.LoadFloat64x8Slice(s[24:]))
		acc4 = acc4.Add(archsimd.LoadFloat64x8Slice(s[32:]))
		acc5 = acc5.Add(archsimd.LoadFloat64x8Slice(s[40:]))
		acc6 = acc6.Add(archsimd.LoadFloat64x8Slice(s[48:]))
		acc7 = acc7.Add(archsimd.LoadFloat64x8Slice(s[56:]))
	}

	acc01 := acc0.Add(acc1)
	acc23 := acc2.Add(acc3)
	acc45 := acc4.Add(acc5)
	acc67 := acc6.Add(acc7)
	acc0123 := acc01.Add(acc23)
	acc4567 := acc45.Add(acc67)
	acc := acc0123.Add(acc4567)

	accArray := [8]float64{}
	acc.Store(&accArray)

	sum := 0.0
	for _, a := range accArray {
		sum += a
	}
	for ; i < n; i++ {
		sum += src[i]
	}
	dst[0] = sum
}

// avx2SumInt32 computes dst[0] = sum(src[i]) using AVX2 (256-bit, 8 int32/vector).
// Uses 8 accumulators with a scalar tail for the final 0-63 elements.
func avx2SumInt32(dst, src []int32) {
	n := len(src)
	i := 0
	numAccumulators := 8
	stride := numAccumulators * 8

	acc0, acc1, acc2, acc3 := archsimd.Int32x8{}, archsimd.Int32x8{}, archsimd.Int32x8{}, archsimd.Int32x8{}
	acc4, acc5, acc6, acc7 := archsimd.Int32x8{}, archsimd.Int32x8{}, archsimd.Int32x8{}, archsimd.Int32x8{}

	for ; i+stride <= n; i += stride {
		s := src[i : i+stride]
		acc0 = acc0.Add(archsimd.LoadInt32x8Slice(s[0:]))
		acc1 = acc1.Add(archsimd.LoadInt32x8Slice(s[8:]))
		acc2 = acc2.Add(archsimd.LoadInt32x8Slice(s[16:]))
		acc3 = acc3.Add(archsimd.LoadInt32x8Slice(s[24:]))
		acc4 = acc4.Add(archsimd.LoadInt32x8Slice(s[32:]))
		acc5 = acc5.Add(archsimd.LoadInt32x8Slice(s[40:]))
		acc6 = acc6.Add(archsimd.LoadInt32x8Slice(s[48:]))
		acc7 = acc7.Add(archsimd.LoadInt32x8Slice(s[56:]))
	}

	acc01 := acc0.Add(acc1)
	acc23 := acc2.Add(acc3)
	acc45 := acc4.Add(acc5)
	acc67 := acc6.Add(acc7)
	acc0123 := acc01.Add(acc23)
	acc4567 := acc45.Add(acc67)
	acc := acc0123.Add(acc4567)

	accArray := [8]int32{}
	acc.Store(&accArray)

	sum := int32(0)
	for _, a := range accArray {
		sum += a
	}
	for ; i < n; i++ {
		sum += src[i]
	}
	dst[0] = sum
}

// avx512SumInt32 computes dst[0] = sum(src[i]) using AVX-512 (512-bit, 16 int32/vector).
// Uses 8 accumulators with a scalar tail for the final 0-127 elements.
func avx512SumInt32(dst, src []int32) {
	n := len(src)
	i := 0
	numAccumulators := 8
	stride := numAccumulators * 16

	acc0, acc1, acc2, acc3 := archsimd.Int32x16{}, archsimd.Int32x16{}, archsimd.Int32x16{}, archsimd.Int32x16{}
	acc4, acc5, acc6, acc7 := archsimd.Int32x16{}, archsimd.Int32x16{}, archsimd.Int32x16{}, archsimd.Int32x16{}

	for ; i+stride <= n; i += stride {
		s := src[i : i+stride]
		acc0 = acc0.Add(archsimd.LoadInt32x16Slice(s[0:]))
		acc1 = acc1.Add(archsimd.LoadInt32x16Slice(s[16:]))
		acc2 = acc2.Add(archsimd.LoadInt32x16Slice(s[32:]))
		acc3 = acc3.Add(archsimd.LoadInt32x16Slice(s[48:]))
		acc4 = acc4.Add(archsimd.LoadInt32x16Slice(s[64:]))
		acc5 = acc5.Add(archsimd.LoadInt32x16Slice(s[80:]))
		acc6 = acc6.Add(archsimd.LoadInt32x16Slice(s[96:]))
		acc7 = acc7.Add(archsimd.LoadInt32x16Slice(s[112:]))
	}

	acc01 := acc0.Add(acc1)
	acc23 := acc2.Add(acc3)
	acc45 := acc4.Add(acc5)
	acc67 := acc6.Add(acc7)
	acc0123 := acc01.Add(acc23)
	acc4567 := acc45.Add(acc67)
	acc := acc0123.Add(acc4567)

	accArray := [16]int32{}
	acc.Store(&accArray)

	sum := int32(0)
	for _, a := range accArray {
		sum += a
	}
	for ; i < n; i++ {
		sum += src[i]
	}
	dst[0] = sum
}

// avx2SumInt64 computes dst[0] = sum(src[i]) using AVX2 (256-bit, 4 int64/vector).
// Uses 8 accumulators with a scalar tail for the final 0-31 elements.
func avx2SumInt64(dst, src []int64) {
	n := len(src)
	i := 0
	numAccumulators := 8
	stride := numAccumulators * 4

	acc0, acc1, acc2, acc3 := archsimd.Int64x4{}, archsimd.Int64x4{}, archsimd.Int64x4{}, archsimd.Int64x4{}
	acc4, acc5, acc6, acc7 := archsimd.Int64x4{}, archsimd.Int64x4{}, archsimd.Int64x4{}, archsimd.Int64x4{}

	for ; i+stride <= n; i += stride {
		s := src[i : i+stride]
		acc0 = acc0.Add(archsimd.LoadInt64x4Slice(s[0:]))
		acc1 = acc1.Add(archsimd.LoadInt64x4Slice(s[4:]))
		acc2 = acc2.Add(archsimd.LoadInt64x4Slice(s[8:]))
		acc3 = acc3.Add(archsimd.LoadInt64x4Slice(s[12:]))
		acc4 = acc4.Add(archsimd.LoadInt64x4Slice(s[16:]))
		acc5 = acc5.Add(archsimd.LoadInt64x4Slice(s[20:]))
		acc6 = acc6.Add(archsimd.LoadInt64x4Slice(s[24:]))
		acc7 = acc7.Add(archsimd.LoadInt64x4Slice(s[28:]))
	}

	acc01 := acc0.Add(acc1)
	acc23 := acc2.Add(acc3)
	acc45 := acc4.Add(acc5)
	acc67 := acc6.Add(acc7)
	acc0123 := acc01.Add(acc23)
	acc4567 := acc45.Add(acc67)
	acc := acc0123.Add(acc4567)

	accArray := [4]int64{}
	acc.Store(&accArray)

	sum := int64(0)
	for _, a := range accArray {
		sum += a
	}
	for ; i < n; i++ {
		sum += src[i]
	}
	dst[0] = sum
}

// avx512SumInt64 computes dst[0] = sum(src[i]) using AVX-512 (512-bit, 8 int64/vector).
// Uses 8 accumulators with a scalar tail for the final 0-63 elements.
func avx512SumInt64(dst, src []int64) {
	n := len(src)
	i := 0
	numAccumulators := 8
	stride := numAccumulators * 8

	acc0, acc1, acc2, acc3 := archsimd.Int64x8{}, archsimd.Int64x8{}, archsimd.Int64x8{}, archsimd.Int64x8{}
	acc4, acc5, acc6, acc7 := archsimd.Int64x8{}, archsimd.Int64x8{}, archsimd.Int64x8{}, archsimd.Int64x8{}

	for ; i+stride <= n; i += stride {
		s := src[i : i+stride]
		acc0 = acc0.Add(archsimd.LoadInt64x8Slice(s[0:]))
		acc1 = acc1.Add(archsimd.LoadInt64x8Slice(s[8:]))
		acc2 = acc2.Add(archsimd.LoadInt64x8Slice(s[16:]))
		acc3 = acc3.Add(archsimd.LoadInt64x8Slice(s[24:]))
		acc4 = acc4.Add(archsimd.LoadInt64x8Slice(s[32:]))
		acc5 = acc5.Add(archsimd.LoadInt64x8Slice(s[40:]))
		acc6 = acc6.Add(archsimd.LoadInt64x8Slice(s[48:]))
		acc7 = acc7.Add(archsimd.LoadInt64x8Slice(s[56:]))
	}

	acc01 := acc0.Add(acc1)
	acc23 := acc2.Add(acc3)
	acc45 := acc4.Add(acc5)
	acc67 := acc6.Add(acc7)
	acc0123 := acc01.Add(acc23)
	acc4567 := acc45.Add(acc67)
	acc := acc0123.Add(acc4567)

	accArray := [8]int64{}
	acc.Store(&accArray)

	sum := int64(0)
	for _, a := range accArray {
		sum += a
	}
	for ; i < n; i++ {
		sum += src[i]
	}
	dst[0] = sum
}
