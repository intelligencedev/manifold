package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// BatchMatMulOp represents a batched matrix multiplication operation: output = a @ b.
//
// Backward pass:
//   - d(A@B)/dA = outputGrad @ B^T
//   - d(A@B)/dB = A^T @ outputGrad
//
// Where @ denotes batched matrix multiplication and ^T denotes transpose.
type BatchMatMulOp struct {
	inputs []*tensor.RawTensor // [a, b]
	output *tensor.RawTensor   // a @ b
}

// NewBatchMatMulOp creates a new BatchMatMulOp.
func NewBatchMatMulOp(a, b, output *tensor.RawTensor) *BatchMatMulOp {
	return &BatchMatMulOp{
		inputs: []*tensor.RawTensor{a, b},
		output: output,
	}
}

// Backward computes gradients for batch matmul.
// Given C = A @ B:
//
//	dL/dA = dL/dC @ B^T
//	dL/dB = A^T @ dL/dC
func (op *BatchMatMulOp) Backward(grad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	a, b := op.inputs[0], op.inputs[1]

	// Get batch transpose helper
	transposeBackend, ok := backend.(interface {
		Transpose(*tensor.RawTensor, ...int) *tensor.RawTensor
		BatchMatMul(*tensor.RawTensor, *tensor.RawTensor) *tensor.RawTensor
	})
	if !ok {
		panic("BatchMatMulOp.Backward: backend must support Transpose and BatchMatMul")
	}

	ndim := len(a.Shape())

	// Build transpose axes for swapping last two dims
	axes := make([]int, ndim)
	for i := 0; i < ndim-2; i++ {
		axes[i] = i
	}
	axes[ndim-2] = ndim - 1
	axes[ndim-1] = ndim - 2

	// B^T: swap last two dimensions
	bT := transposeBackend.Transpose(b, axes...)
	// A^T: swap last two dimensions
	aT := transposeBackend.Transpose(a, axes...)

	// dL/dA = dL/dC @ B^T
	gradA := transposeBackend.BatchMatMul(grad, bT)
	// dL/dB = A^T @ dL/dC
	gradB := transposeBackend.BatchMatMul(aT, grad)

	return []*tensor.RawTensor{gradA, gradB}
}

// Inputs returns the input tensors [a, b].
func (op *BatchMatMulOp) Inputs() []*tensor.RawTensor {
	return op.inputs
}

// Output returns the output tensor a @ b.
func (op *BatchMatMulOp) Output() *tensor.RawTensor {
	return op.output
}
