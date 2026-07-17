package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// SigmoidOp represents the sigmoid activation operation: σ(x) = 1 / (1 + exp(-x)).
type SigmoidOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor
}

// NewSigmoidOp creates a new sigmoid operation.
func NewSigmoidOp(input, output *tensor.RawTensor) *SigmoidOp {
	return &SigmoidOp{
		input:  input,
		output: output,
	}
}

// Inputs returns the input tensors.
func (op *SigmoidOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *SigmoidOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes the gradient for sigmoid.
//
// For σ(x) = 1 / (1 + exp(-x)):
// dσ/dx = σ(x) * (1 - σ(x))
//
// Since we have the output σ(x) already computed, we can use it:
// grad_input = grad_output * output * (1 - output).
func (op *SigmoidOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	output := op.output

	// 1 - σ(x) via backend scalar ops (zero CPU allocation, ADR-009).
	oneMinusSigmoid := backend.MulScalar(backend.AddScalar(output, typedScalar(output.DType(), -1.0)), typedScalar(output.DType(), -1.0))

	// σ(x) * (1 - σ(x))
	sigmoidDerivative := backend.Mul(output, oneMinusSigmoid)

	// grad_input = grad_output * σ'(x)
	inputGrad := backend.Mul(outputGrad, sigmoidDerivative)

	return []*tensor.RawTensor{inputGrad}
}
