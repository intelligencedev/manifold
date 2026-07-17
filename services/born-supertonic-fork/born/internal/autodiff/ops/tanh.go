package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// TanhOp represents the hyperbolic tangent activation: tanh(x) = (exp(x) - exp(-x)) / (exp(x) + exp(-x)).
type TanhOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor
}

// NewTanhOp creates a new tanh operation.
func NewTanhOp(input, output *tensor.RawTensor) *TanhOp {
	return &TanhOp{
		input:  input,
		output: output,
	}
}

// Inputs returns the input tensors.
func (op *TanhOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *TanhOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes the gradient for tanh.
//
// For tanh(x):
// d(tanh(x))/dx = 1 - tanh²(x)
//
// Since we have the output tanh(x) already computed:
// grad_input = grad_output * (1 - output²).
func (op *TanhOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	output := op.output

	// tanh²(x)
	outputSquared := backend.Mul(output, output)

	// 1 - tanh²(x) via backend scalar ops (zero CPU allocation, ADR-009).
	tanhDerivative := backend.MulScalar(backend.AddScalar(outputSquared, typedScalar(output.DType(), -1.0)), typedScalar(output.DType(), -1.0))

	// grad_input = grad_output * (1 - tanh²(x))
	inputGrad := backend.Mul(outputGrad, tanhDerivative)

	return []*tensor.RawTensor{inputGrad}
}
