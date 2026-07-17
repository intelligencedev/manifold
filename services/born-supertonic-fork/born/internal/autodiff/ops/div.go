package ops

import "github.com/born-ml/born/internal/tensor"

// DivOp represents an element-wise division operation: output = a / b.
//
// Backward pass:
//   - d(a/b)/da = 1/b, so grad_a = outputGrad / b
//   - d(a/b)/db = -a/b², so grad_b = -outputGrad * a / b²
type DivOp struct {
	inputs []*tensor.RawTensor // [a, b]
	output *tensor.RawTensor   // a / b
}

// NewDivOp creates a new DivOp.
func NewDivOp(a, b, output *tensor.RawTensor) *DivOp {
	return &DivOp{
		inputs: []*tensor.RawTensor{a, b},
		output: output,
	}
}

// Backward computes input gradients for division.
func (op *DivOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	a, b := op.inputs[0], op.inputs[1]

	// grad_a = outputGrad / b
	gradA := backend.Div(outputGrad, b)
	gradA = reduceBroadcast(gradA, a.Shape(), backend)

	// grad_b = -outputGrad * a / b²
	// = -(outputGrad * a) / (b * b)
	bSquared := backend.Mul(b, b)
	numerator := backend.Mul(outputGrad, a)
	gradB := backend.Div(numerator, bSquared)
	gradB = negateGradient(gradB, backend)
	gradB = reduceBroadcast(gradB, b.Shape(), backend)

	return []*tensor.RawTensor{gradA, gradB}
}

// Inputs returns the input tensors [a, b].
func (op *DivOp) Inputs() []*tensor.RawTensor {
	return op.inputs
}

// Output returns the output tensor a / b.
func (op *DivOp) Output() *tensor.RawTensor {
	return op.output
}
