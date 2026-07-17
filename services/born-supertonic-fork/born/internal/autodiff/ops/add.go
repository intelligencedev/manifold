package ops

import "github.com/born-ml/born/internal/tensor"

// AddOp represents an element-wise addition operation: output = a + b.
//
// Backward pass:
//   - d(a+b)/da = 1, so grad_a = outputGrad
//   - d(a+b)/db = 1, so grad_b = outputGrad
//
// Note: If broadcasting was used in forward pass, gradients must be
// reduced (summed) along the broadcast dimensions to match input shapes.
type AddOp struct {
	inputs []*tensor.RawTensor // [a, b]
	output *tensor.RawTensor   // a + b
}

// NewAddOp creates a new AddOp.
func NewAddOp(a, b, output *tensor.RawTensor) *AddOp {
	return &AddOp{
		inputs: []*tensor.RawTensor{a, b},
		output: output,
	}
}

// Backward computes input gradients for addition.
// Since d(a+b)/da = d(a+b)/db = 1, the gradient flows equally to both inputs.
func (op *AddOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	a, b := op.inputs[0], op.inputs[1]

	// Handle broadcasting: reduce gradient along broadcast dimensions
	gradA := reduceBroadcast(outputGrad, a.Shape(), backend)
	gradB := reduceBroadcast(outputGrad, b.Shape(), backend)

	return []*tensor.RawTensor{gradA, gradB}
}

// Inputs returns the input tensors [a, b].
func (op *AddOp) Inputs() []*tensor.RawTensor {
	return op.inputs
}

// Output returns the output tensor a + b.
func (op *AddOp) Output() *tensor.RawTensor {
	return op.output
}
