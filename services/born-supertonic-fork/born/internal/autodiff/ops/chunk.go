package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// ChunkOp represents a chunk operation that splits a tensor into n equal parts.
//
// Forward: outputs = Chunk(input, n, dim)
//
// Backward:
//
//	Concatenate all output gradients back together along dim.
//	gradInput = Cat([gradOutput1, gradOutput2, ...], dim)
//
// Example:
//
//	input: [1,2,3,4,5,6] along dim=0, n=3
//	outputs: [[1,2], [3,4], [5,6]]
//	gradOutputs: [[dL/d1, dL/d2], [dL/d3, dL/d4], [dL/d5, dL/d6]]
//	gradInput: [dL/d1, dL/d2, dL/d3, dL/d4, dL/d5, dL/d6]
type ChunkOp struct {
	input   *tensor.RawTensor   // Input tensor that was chunked
	n       int                 // Number of chunks
	dim     int                 // Dimension along which chunking happened
	outputs []*tensor.RawTensor // Output chunk tensors
}

// NewChunkOp creates a new chunk operation.
func NewChunkOp(input *tensor.RawTensor, n, dim int, outputs []*tensor.RawTensor) *ChunkOp {
	return &ChunkOp{
		input:   input,
		n:       n,
		dim:     dim,
		outputs: outputs,
	}
}

// Inputs returns the input tensor.
func (op *ChunkOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the first output tensor.
// Note: ChunkOp has multiple outputs, but the Operation interface expects a single output.
// We return the first chunk here, but the tape needs special handling for multi-output ops.
func (op *ChunkOp) Output() *tensor.RawTensor {
	return op.outputs[0]
}

// Outputs returns all output tensors (implements MultiOutputOperation).
func (op *ChunkOp) Outputs() []*tensor.RawTensor {
	return op.outputs
}

// Backward computes gradients for the input tensor.
//
// Since Chunk splits a tensor, the backward pass concatenates the gradients
// of all output chunks back together.
//
// Algorithm:
//  1. Collect gradients for all output chunks
//  2. Concatenate them along the same dimension
//  3. Return the concatenated gradient
//
// Note: The caller must provide gradients for all outputs.
func (op *ChunkOp) Backward(_ *tensor.RawTensor, _ tensor.Backend) []*tensor.RawTensor {
	// For chunk backward, we need gradients for ALL outputs, not just one.
	// This is a limitation of the current Operation interface.
	// For now, we assume the tape will handle multi-output operations specially.
	//
	// If only one gradient is provided, we can't properly compute the backward pass.
	// The proper way would be to have BackwardMulti that accepts []*RawTensor gradOutputs.
	//
	// As a workaround, we'll panic if this is called directly.
	// The tape should use a special path for multi-output operations.
	panic("ChunkOp.Backward: multi-output operations require special handling in tape")
}

// BackwardMulti computes gradients for the input tensor given all output gradients.
//
// This is the proper backward pass for chunk that takes all output gradients.
func (op *ChunkOp) BackwardMulti(gradOutputs []*tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	if len(gradOutputs) != op.n {
		panic("ChunkOp.BackwardMulti: expected n gradients for n outputs")
	}

	// Concatenate all output gradients along the same dimension
	gradInput := backend.Cat(gradOutputs, op.dim)

	return []*tensor.RawTensor{gradInput}
}
