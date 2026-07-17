package ops

import "github.com/born-ml/born/internal/tensor"

// SinOp represents the sine operation: y = sin(x).
//
// Backward pass:
//   - d(sin(x))/dx = cos(x)
//   - grad_input = grad_output * cos(input)
type SinOp struct {
	input  *tensor.RawTensor // x
	output *tensor.RawTensor // sin(x)
}

// NewSinOp creates a new SinOp.
func NewSinOp(input, output *tensor.RawTensor) *SinOp {
	return &SinOp{
		input:  input,
		output: output,
	}
}

// Backward computes input gradient for sin.
//
// Since d(sin(x))/dx = cos(x):
// grad_input = grad_output * cos(input).
func (op *SinOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Compute cos(input)
	cosInput := backend.Cos(op.input)

	// grad_input = grad_output * cos(input)
	gradInput := backend.Mul(outputGrad, cosInput)

	return []*tensor.RawTensor{gradInput}
}

// Inputs returns the input tensor [x].
func (op *SinOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor sin(x).
func (op *SinOp) Output() *tensor.RawTensor {
	return op.output
}
