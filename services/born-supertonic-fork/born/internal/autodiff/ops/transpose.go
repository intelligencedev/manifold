package ops

import "github.com/born-ml/born/internal/tensor"

// TransposeOp represents a transpose operation.
//
// Forward:
//
//	output = transpose(input, axes)
//
// Backward:
//
//	∂L/∂input = transpose(∂L/∂output, inverse_axes)
//
// The gradient of transpose is transpose with inverse axes.
type TransposeOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor
	axes   []int // Axes used for forward transpose
}

// NewTransposeOp creates a new TransposeOp.
func NewTransposeOp(input, output *tensor.RawTensor, axes []int) *TransposeOp {
	return &TransposeOp{
		input:  input,
		output: output,
		axes:   axes,
	}
}

// Backward computes input gradient for transpose.
//
// The gradient of transpose is transpose with inverted axes.
// For example, if forward uses axes [1, 0] (swap), then backward also uses [1, 0].
func (op *TransposeOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Compute inverse axes
	ndim := len(op.axes)
	inverseAxes := make([]int, ndim)
	for i, ax := range op.axes {
		inverseAxes[ax] = i
	}

	// Transpose the output gradient with inverse axes
	inputGrad := backend.Transpose(outputGrad, inverseAxes...)

	return []*tensor.RawTensor{inputGrad}
}

// Inputs returns the input tensors.
func (op *TransposeOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *TransposeOp) Output() *tensor.RawTensor {
	return op.output
}
