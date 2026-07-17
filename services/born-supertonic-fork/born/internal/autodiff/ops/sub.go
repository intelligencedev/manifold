package ops

import "github.com/born-ml/born/internal/tensor"

// SubOp represents an element-wise subtraction operation: output = a - b.
//
// Backward pass:
//   - d(a-b)/da = 1, so grad_a = outputGrad
//   - d(a-b)/db = -1, so grad_b = -outputGrad
type SubOp struct {
	inputs []*tensor.RawTensor // [a, b]
	output *tensor.RawTensor   // a - b
}

// NewSubOp creates a new SubOp.
func NewSubOp(a, b, output *tensor.RawTensor) *SubOp {
	return &SubOp{
		inputs: []*tensor.RawTensor{a, b},
		output: output,
	}
}

// Backward computes input gradients for subtraction.
func (op *SubOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	a, b := op.inputs[0], op.inputs[1]

	// grad_a = outputGrad
	gradA := reduceBroadcast(outputGrad, a.Shape(), backend)

	// grad_b = -outputGrad
	negOutputGrad := negateGradient(outputGrad, backend)
	gradB := reduceBroadcast(negOutputGrad, b.Shape(), backend)

	return []*tensor.RawTensor{gradA, gradB}
}

// Inputs returns the input tensors [a, b].
func (op *SubOp) Inputs() []*tensor.RawTensor {
	return op.inputs
}

// Output returns the output tensor a - b.
func (op *SubOp) Output() *tensor.RawTensor {
	return op.output
}
