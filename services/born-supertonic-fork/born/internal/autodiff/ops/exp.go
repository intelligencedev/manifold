package ops

import "github.com/born-ml/born/internal/tensor"

// ExpOp represents the exponential operation: y = exp(x).
//
// Backward pass:
//   - d(exp(x))/dx = exp(x) = y
//   - grad_input = grad_output * output
type ExpOp struct {
	input  *tensor.RawTensor // x
	output *tensor.RawTensor // exp(x)
}

// NewExpOp creates a new ExpOp.
func NewExpOp(input, output *tensor.RawTensor) *ExpOp {
	return &ExpOp{
		input:  input,
		output: output,
	}
}

// Backward computes input gradient for exp.
//
// Since d(exp(x))/dx = exp(x), and we already have exp(x) as output:
// grad_input = grad_output * output.
func (op *ExpOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// grad_input = grad_output * exp(x)
	gradInput := backend.Mul(outputGrad, op.output)
	return []*tensor.RawTensor{gradInput}
}

// Inputs returns the input tensor [x].
func (op *ExpOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor exp(x).
func (op *ExpOp) Output() *tensor.RawTensor {
	return op.output
}
