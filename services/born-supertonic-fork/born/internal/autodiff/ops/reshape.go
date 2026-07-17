package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// ReshapeOp records a reshape operation for autodiff.
//
// Forward: output = Reshape(input, newShape)
//
// Backward:
//   - d_input: Reshape(d_output, input.shape())
//
// Reshape backward is simple: reshape the output gradient
// back to the original input shape.
type ReshapeOp struct {
	input     *tensor.RawTensor
	output    *tensor.RawTensor
	origShape tensor.Shape
}

// NewReshapeOp creates a new Reshape operation.
func NewReshapeOp(input, output *tensor.RawTensor) *ReshapeOp {
	return &ReshapeOp{
		input:     input,
		output:    output,
		origShape: input.Shape(),
	}
}

// Inputs returns the input tensors.
func (op *ReshapeOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *ReshapeOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes gradients for Reshape.
//
// The gradient of reshape is simple: reshape the output gradient
// back to the input shape. No actual computation needed.
func (op *ReshapeOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Reshape outputGrad back to original input shape
	inputGrad := backend.Reshape(outputGrad, op.origShape)
	return []*tensor.RawTensor{inputGrad}
}
