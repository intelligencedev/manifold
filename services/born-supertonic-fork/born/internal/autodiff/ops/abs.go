package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// AbsOp represents an element-wise absolute value operation: output = abs(a).
//
// Backward pass:
//   - d(abs(a))/da = sign(a), so grad_a = outputGrad * sign(a)
type AbsOp struct {
	input  *tensor.RawTensor // a
	output *tensor.RawTensor // abs(a)
}

// NewAbsOp creates a new AbsOp.
func NewAbsOp(a, output *tensor.RawTensor) *AbsOp {
	return &AbsOp{
		input:  a,
		output: output,
	}
}

// Backward computes input gradients for abs.
func (op *AbsOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	a := op.input

	signA := backend.Sign(a)
	gradA := backend.Mul(outputGrad, signA)

	return []*tensor.RawTensor{gradA}
}

// Inputs returns the input tensor [a].
func (op *AbsOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor abs(a).
func (op *AbsOp) Output() *tensor.RawTensor {
	return op.output
}
