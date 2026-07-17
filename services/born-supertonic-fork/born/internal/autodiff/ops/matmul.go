package ops

import "github.com/born-ml/born/internal/tensor"

// MatMulOp represents a matrix multiplication operation: output = a @ b.
//
// Backward pass:
//   - d(A@B)/dA = outputGrad @ B^T
//   - d(A@B)/dB = A^T @ outputGrad
//
// Where @ denotes matrix multiplication and ^T denotes transpose.
type MatMulOp struct {
	inputs []*tensor.RawTensor // [a, b]
	output *tensor.RawTensor   // a @ b
}

// NewMatMulOp creates a new MatMulOp.
func NewMatMulOp(a, b, output *tensor.RawTensor) *MatMulOp {
	return &MatMulOp{
		inputs: []*tensor.RawTensor{a, b},
		output: output,
	}
}

// Backward computes input gradients for matrix multiplication.
func (op *MatMulOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	a, b := op.inputs[0], op.inputs[1]

	// grad_a = outputGrad @ b^T
	bT := backend.Transpose(b, 1, 0)
	gradA := backend.MatMul(outputGrad, bT)

	// grad_b = a^T @ outputGrad
	aT := backend.Transpose(a, 1, 0)
	gradB := backend.MatMul(aT, outputGrad)

	return []*tensor.RawTensor{gradA, gradB}
}

// Inputs returns the input tensors [a, b].
func (op *MatMulOp) Inputs() []*tensor.RawTensor {
	return op.inputs
}

// Output returns the output tensor a @ b.
func (op *MatMulOp) Output() *tensor.RawTensor {
	return op.output
}
