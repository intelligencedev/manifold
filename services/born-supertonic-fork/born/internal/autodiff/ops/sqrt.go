package ops

import "github.com/born-ml/born/internal/tensor"

// SqrtOp represents the square root operation: y = sqrt(x).
//
// Backward pass:
//   - d(sqrt(x))/dx = 1 / (2 * sqrt(x)) = 0.5 / y
//   - grad_input = grad_output * 0.5 / output
type SqrtOp struct {
	input  *tensor.RawTensor // x
	output *tensor.RawTensor // sqrt(x)
}

// NewSqrtOp creates a new SqrtOp.
func NewSqrtOp(input, output *tensor.RawTensor) *SqrtOp {
	return &SqrtOp{
		input:  input,
		output: output,
	}
}

// Backward computes input gradient for sqrt.
//
// Since d(sqrt(x))/dx = 0.5 / sqrt(x), and we have sqrt(x) as output:
// grad_input = grad_output * 0.5 / output.
func (op *SqrtOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Create tensor with 0.5
	half := createScalar(op.output.Shape(), op.output.DType(), 0.5, backend.Device())

	// grad_input = grad_output * 0.5 / output
	temp := backend.Mul(outputGrad, half)
	gradInput := backend.Div(temp, op.output)

	return []*tensor.RawTensor{gradInput}
}

// Inputs returns the input tensor [x].
func (op *SqrtOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor sqrt(x).
func (op *SqrtOp) Output() *tensor.RawTensor {
	return op.output
}
