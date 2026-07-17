package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// CatOp represents a concatenation operation along a dimension.
//
// Forward: output = Cat([input1, input2, ...], dim)
//
// Backward:
//
//	Split gradOutput along dim at input boundaries and distribute to each input.
//	Each input receives the gradient slice corresponding to its contribution.
//
// Example:
//
//	inputs: [[1,2], [3,4,5]] along dim=0
//	output: [[1,2], [3,4,5]] (shape depends on concat)
//	gradOutput: [dL/d1, dL/d2, dL/d3, dL/d4, dL/d5]
//	gradInput1: [dL/d1, dL/d2]
//	gradInput2: [dL/d3, dL/d4, dL/d5]
type CatOp struct {
	inputs []*tensor.RawTensor // Input tensors that were concatenated
	dim    int                 // Dimension along which concatenation happened
	sizes  []int               // Size of each input along concat dimension
	output *tensor.RawTensor   // Concatenated output tensor
}

// NewCatOp creates a new cat operation.
func NewCatOp(inputs []*tensor.RawTensor, dim int, sizes []int, output *tensor.RawTensor) *CatOp {
	return &CatOp{
		inputs: inputs,
		dim:    dim,
		sizes:  sizes,
		output: output,
	}
}

// Inputs returns the input tensors.
func (op *CatOp) Inputs() []*tensor.RawTensor {
	return op.inputs
}

// Output returns the output tensor.
func (op *CatOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes gradients for the input tensors.
//
// The gradient is split along the concatenation dimension based on the original
// input sizes. Each input receives its corresponding slice of the output gradient.
//
// Algorithm:
//  1. Split gradOutput along dim at boundaries defined by sizes
//  2. Return one gradient slice per input tensor
func (op *CatOp) Backward(gradOutput *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	grads := make([]*tensor.RawTensor, len(op.inputs))

	// Split gradOutput along dim into len(inputs) parts
	// We need to manually slice the gradient tensor
	gradShape := gradOutput.Shape()
	ndim := len(gradShape)

	// Normalize dimension
	dim := op.dim
	if dim < 0 {
		dim = ndim + dim
	}

	// Calculate strides for gradOutput
	gradStrides := gradShape.ComputeStrides()

	// For each input, create a gradient tensor by slicing gradOutput
	offset := 0
	for i, size := range op.sizes {
		// Create gradient tensor with same shape as input[i]
		gradInputShape := gradShape.Clone()
		gradInputShape[dim] = size

		gradInput, err := tensor.NewRaw(gradInputShape, gradOutput.DType(), backend.Device())
		if err != nil {
			panic(err)
		}

		// Copy the slice from gradOutput to gradInput
		copySliceAlongDim(gradInput, gradOutput, dim, offset, gradStrides)

		grads[i] = gradInput
		offset += size
	}

	return grads
}

// copySliceAlongDim copies a slice from src to dst along a specific dimension.
//
// Parameters:
//   - dst: destination tensor
//   - src: source tensor
//   - dim: dimension to slice along
//   - offset: starting position along dim in src
//   - srcStrides: precomputed strides for src
func copySliceAlongDim(dst, src *tensor.RawTensor, dim, offset int, srcStrides []int) {
	dstShape := dst.Shape()
	dstStrides := dstShape.ComputeStrides()
	numElements := dstShape.NumElements()

	// Handle different data types
	switch src.DType() {
	case tensor.Float32:
		copySliceFloat32(dst.AsFloat32(), src.AsFloat32(), dim, offset, dstShape, dstStrides, srcStrides, numElements)
	case tensor.Float64:
		copySliceFloat64(dst.AsFloat64(), src.AsFloat64(), dim, offset, dstShape, dstStrides, srcStrides, numElements)
	case tensor.Int32:
		copySliceInt32(dst.AsInt32(), src.AsInt32(), dim, offset, dstShape, dstStrides, srcStrides, numElements)
	case tensor.Int64:
		copySliceInt64(dst.AsInt64(), src.AsInt64(), dim, offset, dstShape, dstStrides, srcStrides, numElements)
	case tensor.Uint8:
		copySliceUint8(dst.AsUint8(), src.AsUint8(), dim, offset, dstShape, dstStrides, srcStrides, numElements)
	case tensor.Bool:
		copySliceBool(dst.AsBool(), src.AsBool(), dim, offset, dstShape, dstStrides, srcStrides, numElements)
	default:
		panic("copySliceAlongDim: unsupported dtype")
	}
}

// copySliceFloat32 copies float32 data along a dimension.
func copySliceFloat32(dst, src []float32, dim, offset int, dstShape tensor.Shape, dstStrides, srcStrides []int, numElements int) {
	for i := 0; i < numElements; i++ {
		// Compute multi-dimensional index for dst
		temp := i
		srcIdx := 0
		for d := 0; d < len(dstShape); d++ {
			coord := temp / dstStrides[d]
			temp %= dstStrides[d]

			// Add offset for the concat dimension
			if d == dim {
				coord += offset
			}
			srcIdx += coord * srcStrides[d]
		}

		dst[i] = src[srcIdx]
	}
}

// copySliceFloat64 copies float64 data along a dimension.
func copySliceFloat64(dst, src []float64, dim, offset int, dstShape tensor.Shape, dstStrides, srcStrides []int, numElements int) {
	for i := 0; i < numElements; i++ {
		temp := i
		srcIdx := 0
		for d := 0; d < len(dstShape); d++ {
			coord := temp / dstStrides[d]
			temp %= dstStrides[d]

			if d == dim {
				coord += offset
			}
			srcIdx += coord * srcStrides[d]
		}

		dst[i] = src[srcIdx]
	}
}

// copySliceInt32 copies int32 data along a dimension.
func copySliceInt32(dst, src []int32, dim, offset int, dstShape tensor.Shape, dstStrides, srcStrides []int, numElements int) {
	for i := 0; i < numElements; i++ {
		temp := i
		srcIdx := 0
		for d := 0; d < len(dstShape); d++ {
			coord := temp / dstStrides[d]
			temp %= dstStrides[d]

			if d == dim {
				coord += offset
			}
			srcIdx += coord * srcStrides[d]
		}

		dst[i] = src[srcIdx]
	}
}

// copySliceInt64 copies int64 data along a dimension.
func copySliceInt64(dst, src []int64, dim, offset int, dstShape tensor.Shape, dstStrides, srcStrides []int, numElements int) {
	for i := 0; i < numElements; i++ {
		temp := i
		srcIdx := 0
		for d := 0; d < len(dstShape); d++ {
			coord := temp / dstStrides[d]
			temp %= dstStrides[d]

			if d == dim {
				coord += offset
			}
			srcIdx += coord * srcStrides[d]
		}

		dst[i] = src[srcIdx]
	}
}

// copySliceUint8 copies uint8 data along a dimension.
func copySliceUint8(dst, src []uint8, dim, offset int, dstShape tensor.Shape, dstStrides, srcStrides []int, numElements int) {
	for i := 0; i < numElements; i++ {
		temp := i
		srcIdx := 0
		for d := 0; d < len(dstShape); d++ {
			coord := temp / dstStrides[d]
			temp %= dstStrides[d]

			if d == dim {
				coord += offset
			}
			srcIdx += coord * srcStrides[d]
		}

		dst[i] = src[srcIdx]
	}
}

// copySliceBool copies bool data along a dimension.
func copySliceBool(dst, src []bool, dim, offset int, dstShape tensor.Shape, dstStrides, srcStrides []int, numElements int) {
	for i := 0; i < numElements; i++ {
		temp := i
		srcIdx := 0
		for d := 0; d < len(dstShape); d++ {
			coord := temp / dstStrides[d]
			temp %= dstStrides[d]

			if d == dim {
				coord += offset
			}
			srcIdx += coord * srcStrides[d]
		}

		dst[i] = src[srcIdx]
	}
}
