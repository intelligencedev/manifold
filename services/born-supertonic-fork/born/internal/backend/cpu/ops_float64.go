package cpu

import (
	"github.com/born-ml/born/internal/tensor"
)

// Float64 operations follow the same pattern as float32

func addInplaceFloat64(a, b []float64) {
	if simdAddInplaceFloat64 != nil {
		simdAddInplaceFloat64(a, b)
		return
	}
	for i := range a {
		a[i] += b[i]
	}
}

func subInplaceFloat64(a, b []float64) {
	if simdSubInplaceFloat64 != nil {
		simdSubInplaceFloat64(a, b)
		return
	}
	for i := range a {
		a[i] -= b[i]
	}
}

func mulInplaceFloat64(a, b []float64) {
	if simdMulInplaceFloat64 != nil {
		simdMulInplaceFloat64(a, b)
		return
	}
	for i := range a {
		a[i] *= b[i]
	}
}

func divInplaceFloat64(a, b []float64) {
	if simdDivInplaceFloat64 != nil {
		simdDivInplaceFloat64(a, b)
		return
	}
	for i := range a {
		a[i] /= b[i]
	}
}

func addVectorizedFloat64(dst, a, b []float64) {
	if simdAddVectorizedFloat64 != nil {
		simdAddVectorizedFloat64(dst, a, b)
		return
	}
	for i := range a {
		dst[i] = a[i] + b[i]
	}
}

func subVectorizedFloat64(dst, a, b []float64) {
	if simdSubVectorizedFloat64 != nil {
		simdSubVectorizedFloat64(dst, a, b)
		return
	}
	for i := range a {
		dst[i] = a[i] - b[i]
	}
}

func mulVectorizedFloat64(dst, a, b []float64) {
	if simdMulVectorizedFloat64 != nil {
		simdMulVectorizedFloat64(dst, a, b)
		return
	}
	for i := range a {
		dst[i] = a[i] * b[i]
	}
}

func divVectorizedFloat64(dst, a, b []float64) {
	if simdDivVectorizedFloat64 != nil {
		simdDivVectorizedFloat64(dst, a, b)
		return
	}
	for i := range a {
		dst[i] = a[i] / b[i]
	}
}

// The float64 broadcasting ops use the same incremental odometer over the output
// coordinates as their float32 counterparts (see ops_float32.go): each step
// updates the source flat indices with adds and a carry at dimension boundaries,
// avoiding the per-element integer division of computeFlatIndex. Bit-identical to
// the division form.

func addBroadcastFloat64(dst, a, b []float64, aShape, bShape, outShape tensor.Shape) {
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	ndim := len(outShape)
	coords := make([]int, ndim)
	aIdx, bIdx := 0, 0
	for i := 0; i < n; i++ {
		dst[i] = a[aIdx] + b[bIdx]
		for d := ndim - 1; d >= 0; d-- {
			coords[d]++
			aIdx += aStrides[d]
			bIdx += bStrides[d]
			if coords[d] < outShape[d] {
				break
			}
			coords[d] = 0
			aIdx -= outShape[d] * aStrides[d]
			bIdx -= outShape[d] * bStrides[d]
		}
	}
}

func subBroadcastFloat64(dst, a, b []float64, aShape, bShape, outShape tensor.Shape) {
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	ndim := len(outShape)
	coords := make([]int, ndim)
	aIdx, bIdx := 0, 0
	for i := 0; i < n; i++ {
		dst[i] = a[aIdx] - b[bIdx]
		for d := ndim - 1; d >= 0; d-- {
			coords[d]++
			aIdx += aStrides[d]
			bIdx += bStrides[d]
			if coords[d] < outShape[d] {
				break
			}
			coords[d] = 0
			aIdx -= outShape[d] * aStrides[d]
			bIdx -= outShape[d] * bStrides[d]
		}
	}
}

// mulBroadcastFloat64 uses the general incremental odometer only: the structured
// fast paths added for float32 (see mulBroadcastFullFloat32) target the model's
// inference hot path, which is float32. float64 broadcast multiply is rare enough
// that the extra paths are not worth the duplicated complexity here.
func mulBroadcastFloat64(dst, a, b []float64, aShape, bShape, outShape tensor.Shape) {
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	ndim := len(outShape)
	coords := make([]int, ndim)
	aIdx, bIdx := 0, 0
	for i := 0; i < n; i++ {
		dst[i] = a[aIdx] * b[bIdx]
		for d := ndim - 1; d >= 0; d-- {
			coords[d]++
			aIdx += aStrides[d]
			bIdx += bStrides[d]
			if coords[d] < outShape[d] {
				break
			}
			coords[d] = 0
			aIdx -= outShape[d] * aStrides[d]
			bIdx -= outShape[d] * bStrides[d]
		}
	}
}

func divBroadcastFloat64(dst, a, b []float64, aShape, bShape, outShape tensor.Shape) {
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	ndim := len(outShape)
	coords := make([]int, ndim)
	aIdx, bIdx := 0, 0
	for i := 0; i < n; i++ {
		dst[i] = a[aIdx] / b[bIdx]
		for d := ndim - 1; d >= 0; d-- {
			coords[d]++
			aIdx += aStrides[d]
			bIdx += bStrides[d]
			if coords[d] < outShape[d] {
				break
			}
			coords[d] = 0
			aIdx -= outShape[d] * aStrides[d]
			bIdx -= outShape[d] * bStrides[d]
		}
	}
}

func transposeFloat64(dst, src []float64, shape tensor.Shape, axes []int) {
	ndim := len(shape)
	srcStrides := shape.ComputeStrides()

	// Compute destination shape and strides
	dstShape := make(tensor.Shape, ndim)
	for i, ax := range axes {
		dstShape[i] = shape[ax]
	}
	dstStrides := dstShape.ComputeStrides()

	// Transpose data
	n := shape.NumElements()
	for i := 0; i < n; i++ {
		// Compute multi-dimensional coordinates in source
		coords := make([]int, ndim)
		idx := i
		for dim := 0; dim < ndim; dim++ {
			coords[dim] = idx / srcStrides[dim]
			idx %= srcStrides[dim]
		}

		// Permute coordinates according to axes
		permutedCoords := make([]int, ndim)
		for dstDim, srcDim := range axes {
			permutedCoords[dstDim] = coords[srcDim]
		}

		// Compute flat index in destination
		dstIdx := 0
		for dim := 0; dim < ndim; dim++ {
			dstIdx += permutedCoords[dim] * dstStrides[dim]
		}

		dst[dstIdx] = src[i]
	}
}
