package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// ScatterAdd performs a general scatter-add: the inverse of Gather.
//
// For each element at flat position i in src (which has the same shape as indices):
//
//	result[coords_except_dim..., indices[coords...], ...] += src[coords...]
//
// This is used in Gather backward: Gather selects elements along dim using an
// N-D index tensor; ScatterAdd accumulates gradients back to the original positions.
// Semantics follow Burn's float_scatter_add.
//
// Parameters:
//   - dest:    zero-filled base tensor (same shape as the Gather input)
//   - dim:     scatter dimension (negative values wrap around)
//   - indices: int32 tensor with same shape as src; each element is a position in dest along dim
//   - src:     gradient tensor (same shape as the Gather output / indices)
//
// Returns a new tensor with the same shape as dest. dest is not modified.
// indices must have dtype int32.
func (cpu *CPUBackend) ScatterAdd(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) *tensor.RawTensor {
	dim = validateScatterAdd(dest, dim, indices, src)

	destShape := dest.Shape()
	srcShape := src.Shape()
	indexShape := indices.Shape()

	// Clone dest so we do not modify the input.
	result, err := tensor.NewRaw(destShape, dest.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("scatteradd: failed to create result tensor: %v", err))
	}
	copy(result.Data(), dest.Data())

	idxData := indices.AsInt32()
	numElements := src.NumElements()
	ndim := len(destShape)
	srcStrides := srcShape.ComputeStrides()
	dstStrides := destShape.ComputeStrides()
	indexStrides := indexShape.ComputeStrides()

	switch dest.DType() {
	case tensor.Float32:
		scatterAddCPUFloat32(result.AsFloat32(), src.AsFloat32(), idxData,
			dim, numElements, ndim, destShape, srcStrides, dstStrides, indexStrides)
	case tensor.Float64:
		scatterAddCPUFloat64(result.AsFloat64(), src.AsFloat64(), idxData,
			dim, numElements, ndim, destShape, srcStrides, dstStrides, indexStrides)
	case tensor.Int32:
		scatterAddCPUInt32(result.AsInt32(), src.AsInt32(), idxData,
			dim, numElements, ndim, destShape, srcStrides, dstStrides, indexStrides)
	case tensor.Int64:
		scatterAddCPUInt64(result.AsInt64(), src.AsInt64(), idxData,
			dim, numElements, ndim, destShape, srcStrides, dstStrides, indexStrides)
	default:
		panic(fmt.Sprintf("scatteradd: unsupported dtype %s", dest.DType()))
	}

	return result
}

// validateScatterAdd validates all preconditions and returns the normalized dim.
func validateScatterAdd(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) int {
	if indices.DType() != tensor.Int32 {
		panic(fmt.Sprintf("scatteradd: indices must be int32, got %s", indices.DType()))
	}

	destShape := dest.Shape()
	srcShape := src.Shape()
	indexShape := indices.Shape()
	ndim := len(destShape)

	if dim < 0 {
		dim += ndim
	}
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("scatteradd: dim %d out of range for %dD tensor", dim, ndim))
	}

	if len(indexShape) != len(srcShape) {
		panic(fmt.Sprintf("scatteradd: indices rank %d != src rank %d", len(indexShape), len(srcShape)))
	}
	for d := range indexShape {
		if indexShape[d] != srcShape[d] {
			panic(fmt.Sprintf("scatteradd: indices shape %v != src shape %v", indexShape, srcShape))
		}
	}

	if len(srcShape) != ndim {
		panic(fmt.Sprintf("scatteradd: src rank %d != dest rank %d", len(srcShape), ndim))
	}
	for d := 0; d < ndim; d++ {
		if d == dim {
			continue
		}
		if srcShape[d] != destShape[d] {
			panic(fmt.Sprintf("scatteradd: shape mismatch at dim %d: dest=%d src=%d", d, destShape[d], srcShape[d]))
		}
	}

	return dim
}

// scatterCoords decomposes a flat element index into N-D coordinates using strides.
func scatterCoords(flatIdx, ndim int, strides []int) []int {
	coords := make([]int, ndim)
	rem := flatIdx
	for d := 0; d < ndim; d++ {
		coords[d] = rem / strides[d]
		rem %= strides[d]
	}
	return coords
}

// scatterIndexFlat computes a flat index from coords and strides.
func scatterIndexFlat(coords []int, ndim int, strides []int) int {
	idx := 0
	for d := 0; d < ndim; d++ {
		idx += coords[d] * strides[d]
	}
	return idx
}

// scatterDstFlat computes the destination flat index, substituting dim coordinate with idx.
func scatterDstFlat(coords []int, idx, dim, ndim int, dstStrides []int) int {
	dstIdx := 0
	for d := 0; d < ndim; d++ {
		if d == dim {
			dstIdx += idx * dstStrides[d]
		} else {
			dstIdx += coords[d] * dstStrides[d]
		}
	}
	return dstIdx
}

func scatterAddCPUFloat32(dst, src []float32, indices []int32, dim, numElements, ndim int,
	dstShape tensor.Shape, srcStrides, dstStrides, indexStrides []int) {
	for i := 0; i < numElements; i++ {
		coords := scatterCoords(i, ndim, srcStrides)
		idx := int(indices[scatterIndexFlat(coords, ndim, indexStrides)])
		if idx < 0 || idx >= dstShape[dim] {
			panic(fmt.Sprintf("scatteradd: index %d out of bounds [0, %d)", idx, dstShape[dim]))
		}
		dst[scatterDstFlat(coords, idx, dim, ndim, dstStrides)] += src[i]
	}
}

func scatterAddCPUFloat64(dst, src []float64, indices []int32, dim, numElements, ndim int,
	dstShape tensor.Shape, srcStrides, dstStrides, indexStrides []int) {
	for i := 0; i < numElements; i++ {
		coords := scatterCoords(i, ndim, srcStrides)
		idx := int(indices[scatterIndexFlat(coords, ndim, indexStrides)])
		if idx < 0 || idx >= dstShape[dim] {
			panic(fmt.Sprintf("scatteradd: index %d out of bounds [0, %d)", idx, dstShape[dim]))
		}
		dst[scatterDstFlat(coords, idx, dim, ndim, dstStrides)] += src[i]
	}
}

func scatterAddCPUInt32(dst, src, indices []int32, dim, numElements, ndim int,
	dstShape tensor.Shape, srcStrides, dstStrides, indexStrides []int) {
	for i := 0; i < numElements; i++ {
		coords := scatterCoords(i, ndim, srcStrides)
		idx := int(indices[scatterIndexFlat(coords, ndim, indexStrides)])
		if idx < 0 || idx >= dstShape[dim] {
			panic(fmt.Sprintf("scatteradd: index %d out of bounds [0, %d)", idx, dstShape[dim]))
		}
		dst[scatterDstFlat(coords, idx, dim, ndim, dstStrides)] += src[i]
	}
}

func scatterAddCPUInt64(dst, src []int64, indices []int32, dim, numElements, ndim int,
	dstShape tensor.Shape, srcStrides, dstStrides, indexStrides []int) {
	for i := 0; i < numElements; i++ {
		coords := scatterCoords(i, ndim, srcStrides)
		idx := int(indices[scatterIndexFlat(coords, ndim, indexStrides)])
		if idx < 0 || idx >= dstShape[dim] {
			panic(fmt.Sprintf("scatteradd: index %d out of bounds [0, %d)", idx, dstShape[dim]))
		}
		dst[scatterDstFlat(coords, idx, dim, ndim, dstStrides)] += src[i]
	}
}
