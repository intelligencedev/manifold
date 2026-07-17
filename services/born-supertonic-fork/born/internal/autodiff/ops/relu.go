package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// ReLUOp represents a ReLU (Rectified Linear Unit) activation: output = max(0, x).
//
// Backward pass:
//   - d(ReLU(x))/dx = 1 if x > 0, else 0
//
// The gradient is passed through where the input was positive, and blocked
// (zero) everywhere else.
type ReLUOp struct {
	input  *tensor.RawTensor // x
	output *tensor.RawTensor // max(0, x)
}

// NewReLUOp creates a new ReLUOp.
func NewReLUOp(input, output *tensor.RawTensor) *ReLUOp {
	return &ReLUOp{
		input:  input,
		output: output,
	}
}

// Backward computes input gradient for ReLU using only backend ops (no CPU readback).
//
// d(ReLU(x))/dx = 1 if x > 0, else 0
//
//	zeros = x * 0             (same shape/dtype as x, all zeros)
//	mask  = Greater(x, zeros) (Bool tensor: true where x > 0)
//	grad  = Where(mask, outputGrad, zeros)
//
// The scalar 0 is typed to match the tensor's dtype as required by the backend.
func (op *ReLUOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Typed zero scalar — backend.MulScalar requires scalar type == tensor dtype.
	var zero any
	switch op.input.DType() {
	case tensor.Float32:
		zero = float32(0)
	default: // Float64
		zero = float64(0)
	}

	// zeros: same shape and dtype as input, all elements 0.0 — no data leaves device.
	zeros := backend.MulScalar(op.input, zero)

	// mask[i] = true where input[i] > 0 (returned as tensor.Bool dtype).
	mask := backend.Greater(op.input, zeros)

	// grad_input[i] = outputGrad[i] if mask[i] else 0.0
	gradInput := backend.Where(mask, outputGrad, zeros)

	return []*tensor.RawTensor{gradInput}
}

// Inputs returns the input tensor [x].
func (op *ReLUOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor max(0, x).
func (op *ReLUOp) Output() *tensor.RawTensor {
	return op.output
}
