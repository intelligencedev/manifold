package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// GatherOp represents a gather operation that selects elements along a dimension.
//
// Forward: output = Gather(input, dim, index)
//
// Backward:
//
//	Scatter-add gradOutput to gradInput at positions specified by index.
//	gradInput is initialized to zeros and gradients are accumulated at indexed positions.
//
// Example:
//
//	input: [10, 20, 30, 40]
//	index: [2, 0, 3] along dim=0
//	output: [30, 10, 40]
//	gradOutput: [dL/d30, dL/d10, dL/d40]
//	gradInput: [dL/d10, 0, dL/d30, dL/d40]  (scattered back to original positions)
type GatherOp struct {
	input  *tensor.RawTensor // Input tensor
	dim    int               // Dimension along which gather happened
	index  *tensor.RawTensor // Index tensor (int32)
	output *tensor.RawTensor // Gathered output tensor
}

// NewGatherOp creates a new gather operation.
func NewGatherOp(input *tensor.RawTensor, dim int, index, output *tensor.RawTensor) *GatherOp {
	return &GatherOp{
		input:  input,
		dim:    dim,
		index:  index,
		output: output,
	}
}

// Inputs returns the input tensor.
// Note: index tensor doesn't need gradient.
func (op *GatherOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *GatherOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes gradients for the input tensor.
//
// Gradient computation:
//   - Create gradInput (same shape as input) initialized to zeros (NewRaw zero-initializes)
//   - Scatter-add gradOutput into gradInput at positions specified by op.index
//   - Multiple indices pointing to the same position accumulate gradients
//
// Delegates to backend.ScatterAdd — the general N-D scatter-add that mirrors
// Burn's float_scatter_add. This keeps the backward computation on the accelerator
// (no AsFloat32/AsInt32 readbacks). The index tensor is cast to int32 first if needed,
// since ScatterAdd requires int32 indices.
func (op *GatherOp) Backward(gradOutput *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	inputShape := op.input.Shape()

	// NewRaw zero-initializes (make([]byte, size) in Go is zeroed).
	gradInput, err := tensor.NewRaw(inputShape, gradOutput.DType(), backend.Device())
	if err != nil {
		panic(err)
	}

	// ScatterAdd requires int32 indices. Cast if the stored index has a different dtype.
	// Gather forward accepts int32 and int64; we normalize to int32 here to satisfy the
	// ScatterAdd contract without reading raw data (Cast is a backend op).
	idx := op.index
	if idx.DType() != tensor.Int32 {
		idx = backend.Cast(idx, tensor.Int32)
	}

	gradInput = backend.ScatterAdd(gradInput, op.dim, idx, gradOutput)

	return []*tensor.RawTensor{gradInput}
}
