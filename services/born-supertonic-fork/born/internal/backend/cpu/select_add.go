package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// SelectAdd performs a scatter-add along the specified dimension.
// For each index i in indices, it accumulates src[i, ...] into result[indices[i], ...].
//
// Parameters:
//   - dest: base tensor to accumulate into (shape [V, D] for dim=0)
//   - dim: dimension along which to scatter (0-based, negative values wrap around)
//   - indices: int32 tensor of shape [N] specifying target positions in dest
//   - src: tensor of shape [N, <non-dim dims matching dest>] with values to accumulate
//
// Returns a new tensor with the same shape as dest. dest is not modified.
//
// Primary use case: Embedding backward pass, where gradients for repeated indices
// are accumulated (scatter-add) into the weight gradient tensor.
func (cpu *CPUBackend) SelectAdd(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) *tensor.RawTensor {
	// Validate index dtype.
	if indices.DType() != tensor.Int32 {
		panic(fmt.Sprintf("selectadd: indices must be int32, got %s", indices.DType()))
	}

	destShape := dest.Shape()
	srcShape := src.Shape()
	ndim := len(destShape)

	// Normalize dimension.
	if dim < 0 {
		dim += ndim
	}
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("selectadd: dim %d out of range for %dD tensor", dim, ndim))
	}

	// indices must be 1-D for the scatter dimension.
	if len(indices.Shape()) != 1 {
		panic(fmt.Sprintf("selectadd: indices must be 1-D, got shape %v", indices.Shape()))
	}

	numIndices := indices.Shape()[0]

	// src must have same rank as dest.
	if len(srcShape) != ndim {
		panic(fmt.Sprintf("selectadd: src rank %d != dest rank %d", len(srcShape), ndim))
	}
	// src dim at the scatter axis must equal the number of indices.
	if srcShape[dim] != numIndices {
		panic(fmt.Sprintf("selectadd: src dim %d (%d) != len(indices) (%d)", dim, srcShape[dim], numIndices))
	}
	// All non-scatter dims must match between src and dest.
	for d := 0; d < ndim; d++ {
		if d == dim {
			continue
		}
		if srcShape[d] != destShape[d] {
			panic(fmt.Sprintf("selectadd: shape mismatch at dim %d: dest=%d src=%d", d, destShape[d], srcShape[d]))
		}
	}

	// Clone dest so we do not modify the input.
	result, err := tensor.NewRaw(destShape, dest.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("selectadd: failed to create result tensor: %v", err))
	}
	copy(result.Data(), dest.Data())

	idxData := indices.AsInt32()

	switch dest.DType() {
	case tensor.Float32:
		selectAddFloat32(result.AsFloat32(), idxData, src.AsFloat32(), destShape, srcShape, dim)
	case tensor.Float64:
		selectAddFloat64(result.AsFloat64(), idxData, src.AsFloat64(), destShape, srcShape, dim)
	default:
		panic(fmt.Sprintf("selectadd: unsupported dtype %s", dest.DType()))
	}

	return result
}

// selectAddFloat32 performs the scatter-add loop for float32 data.
//
// For each index i, it accumulates src[i, <rest>] into dst[indices[i], <rest>].
// The implementation handles arbitrary dim values using flat-index arithmetic.
func selectAddFloat32(dst []float32, indices []int32, src []float32, dstShape, srcShape tensor.Shape, dim int) {
	dstStrides := dstShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()
	numIndices := len(indices)

	// innerSize is the number of elements in each "slice" along the scatter axis.
	// For srcShape [N, D] with dim=0, innerSize = D.
	innerSize := srcShape.NumElements() / srcShape[dim]
	srcDimStride := srcStrides[dim]
	dstDimStride := dstStrides[dim]

	for i := 0; i < numIndices; i++ {
		idx := int(indices[i])
		if idx < 0 || idx >= dstShape[dim] {
			panic(fmt.Sprintf("selectadd: index %d out of bounds [0, %d)", idx, dstShape[dim]))
		}

		for j := 0; j < innerSize; j++ {
			// Compute the flat-index contribution from dimensions other than dim,
			// given a linear index j that enumerates elements in those dimensions.
			nonDimFlat := computeNonDimFlat(j, srcShape, srcStrides, dim)

			srcFlat := i*srcDimStride + nonDimFlat
			dstFlat := idx*dstDimStride + nonDimFlat

			dst[dstFlat] += src[srcFlat]
		}
	}
}

// selectAddFloat64 performs the scatter-add loop for float64 data.
func selectAddFloat64(dst []float64, indices []int32, src []float64, dstShape, srcShape tensor.Shape, dim int) {
	dstStrides := dstShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()
	numIndices := len(indices)

	innerSize := srcShape.NumElements() / srcShape[dim]
	srcDimStride := srcStrides[dim]
	dstDimStride := dstStrides[dim]

	for i := 0; i < numIndices; i++ {
		idx := int(indices[i])
		if idx < 0 || idx >= dstShape[dim] {
			panic(fmt.Sprintf("selectadd: index %d out of bounds [0, %d)", idx, dstShape[dim]))
		}

		for j := 0; j < innerSize; j++ {
			nonDimFlat := computeNonDimFlat(j, srcShape, srcStrides, dim)
			srcFlat := i*srcDimStride + nonDimFlat
			dstFlat := idx*dstDimStride + nonDimFlat
			dst[dstFlat] += src[srcFlat]
		}
	}
}

// computeNonDimFlat converts a linear index j (enumerating elements in all dimensions
// except dim) to a flat offset using the full strides of shape.
//
// Example: shape=[N,D1,D2], dim=0 → innerSize=D1*D2, j in [0, D1*D2).
// For j, the coord at d=1 is j/D2 and at d=2 is j%D2; the returned flat offset
// is (j/D2)*strides[1] + (j%D2)*strides[2].
func computeNonDimFlat(j int, shape tensor.Shape, strides []int, dim int) int {
	ndim := len(shape)

	flat := 0
	rem := j
	for d := 0; d < ndim; d++ {
		if d == dim {
			continue
		}
		// Stride of dimension d within the non-dim enumeration (product of non-dim
		// sizes for dimensions after d).
		nonDimStride := 1
		for dd := d + 1; dd < ndim; dd++ {
			if dd != dim {
				nonDimStride *= shape[dd]
			}
		}
		coord := rem / nonDimStride
		rem %= nonDimStride
		flat += coord * strides[d]
	}
	return flat
}
