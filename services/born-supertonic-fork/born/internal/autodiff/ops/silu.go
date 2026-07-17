package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// SiLUOp represents the SiLU (Swish) activation operation: y = x * sigmoid(x).
//
// Also known as Swish activation, widely used in modern transformers
// (LLaMA, Mistral, GPT-Neo).
type SiLUOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor
}

// NewSiLUOp creates a new SiLU operation.
func NewSiLUOp(input, output *tensor.RawTensor) *SiLUOp {
	return &SiLUOp{
		input:  input,
		output: output,
	}
}

// Inputs returns the input tensors.
func (op *SiLUOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *SiLUOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes the gradient for SiLU using only backend ops (no CPU readback).
//
// For y = x * sigmoid(x):
//
//	dy/dx = sigmoid(x) * (1 + x * (1 - sigmoid(x)))
//
// We decompose sigmoid using:
//
//	σ(x)     = 1 / (1 + exp(-x))
//	1 - σ(x) = exp(-x) / (1 + exp(-x))
//
// Both fractions share the same denominator (1 + exp(-x)), so we compute it
// once and use it for both, avoiding any tensor reuse that could trigger the
// CPU backend's in-place Div optimization and corrupt intermediate results.
//
// Scalars are typed to match the tensor's dtype as required by the backend.
func (op *SiLUOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	x := op.input

	// Typed scalars: backend.MulScalar/AddScalar require scalar type == tensor dtype.
	var negOne, zero, one any
	switch x.DType() {
	case tensor.Float32:
		negOne, zero, one = float32(-1), float32(0), float32(1)
	default: // Float64
		negOne, zero, one = float64(-1), float64(0), float64(1)
	}

	// negX = -x
	negX := backend.MulScalar(x, negOne)
	// expNegX = exp(-x)
	expNegX := backend.Exp(negX)
	// denom = 1 + exp(-x)  — used as divisor for both sig and oneMinusSig
	denom := backend.AddScalar(expNegX, one)

	// sig = 1 / (1 + exp(-x)):  fresh ones tensor consumed by Div, denom untouched (b arg).
	ones := backend.AddScalar(backend.MulScalar(x, zero), one)
	sig := backend.Div(ones, denom) // ones is unique → divInplace(ones, denom) → sig

	// 1 - sig = exp(-x) / (1 + exp(-x)):  expNegX consumed by Div, denom still valid.
	// Note: expNegX is a different tensor from ones, so this is safe.
	oneMinusSig := backend.Div(expNegX, denom)

	// Derivative: sigmoid(x) * (1 + x * (1 - sigmoid(x)))
	xOneMinusSig := backend.Mul(x, oneMinusSig)   // x * (1 - σ(x))
	inner := backend.AddScalar(xOneMinusSig, one) // 1 + x*(1-σ(x))
	deriv := backend.Mul(sig, inner)              // σ(x) * inner

	// Chain rule: grad_input = grad_output * derivative
	gradInput := backend.Mul(outputGrad, deriv)

	return []*tensor.RawTensor{gradInput}
}
