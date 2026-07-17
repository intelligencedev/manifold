package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// SignOp represents an element-wise sign operation: output = sign(a).
//
// Backward pass:
//   - d(sign(a))/da = 0 (except at 0, where it's undefined), so grad_a = 0
type SignOp struct {
	input  *tensor.RawTensor // a
	output *tensor.RawTensor // sign(a)
}

// NewSignOp creates a new SignOp.
func NewSignOp(a, output *tensor.RawTensor) *SignOp {
	return &SignOp{
		input:  a,
		output: output,
	}
}

// Backward computes input gradients for sign.
func (op *SignOp) Backward(_ *tensor.RawTensor, _ tensor.Backend) []*tensor.RawTensor {
	a := op.input
	zeros, err := tensor.NewRaw(a.Shape(), a.DType(), a.Device()) // zero tensor with same shape/dtype/device as input
	if err != nil {
		panic(err)
	}
	return []*tensor.RawTensor{zeros}
}

// Inputs returns the input tensor [a].
func (op *SignOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor sign(a).
func (op *SignOp) Output() *tensor.RawTensor {
	return op.output
}
