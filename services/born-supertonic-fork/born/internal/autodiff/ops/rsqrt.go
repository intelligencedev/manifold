package ops

import "github.com/born-ml/born/internal/tensor"

// RsqrtOp represents the reciprocal square root operation: y = 1/sqrt(x).
//
// Backward pass:
//   - d(1/sqrt(x))/dx = -0.5 * x^(-3/2) = -0.5 * (1/sqrt(x))^3 = -0.5 * y^3
//   - grad_input = grad_output * (-0.5) * output^3
type RsqrtOp struct {
	input  *tensor.RawTensor // x
	output *tensor.RawTensor // 1/sqrt(x)
}

// NewRsqrtOp creates a new RsqrtOp.
func NewRsqrtOp(input, output *tensor.RawTensor) *RsqrtOp {
	return &RsqrtOp{
		input:  input,
		output: output,
	}
}

// Backward computes input gradient for rsqrt.
//
// Since d(1/sqrt(x))/dx = -0.5 * y^3, where y = 1/sqrt(x):
// grad_input = grad_output * (-0.5) * output^3.
func (op *RsqrtOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Create tensor with -0.5
	negHalf := createScalar(op.output.Shape(), op.output.DType(), -0.5, backend.Device())

	// output^2
	outputSquared := backend.Mul(op.output, op.output)

	// output^3 = output^2 * output
	outputCubed := backend.Mul(outputSquared, op.output)

	// -0.5 * output^3
	derivative := backend.Mul(negHalf, outputCubed)

	// grad_input = grad_output * derivative
	gradInput := backend.Mul(outputGrad, derivative)

	return []*tensor.RawTensor{gradInput}
}

// Inputs returns the input tensor [x].
func (op *RsqrtOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor 1/sqrt(x).
func (op *RsqrtOp) Output() *tensor.RawTensor {
	return op.output
}
