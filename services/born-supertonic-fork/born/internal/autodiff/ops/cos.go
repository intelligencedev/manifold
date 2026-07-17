package ops

import "github.com/born-ml/born/internal/tensor"

// CosOp represents the cosine operation: y = cos(x).
//
// Backward pass:
//   - d(cos(x))/dx = -sin(x)
//   - grad_input = grad_output * (-sin(input))
type CosOp struct {
	input  *tensor.RawTensor // x
	output *tensor.RawTensor // cos(x)
}

// NewCosOp creates a new CosOp.
func NewCosOp(input, output *tensor.RawTensor) *CosOp {
	return &CosOp{
		input:  input,
		output: output,
	}
}

// Backward computes input gradient for cos.
//
// Since d(cos(x))/dx = -sin(x):
// grad_input = grad_output * (-sin(input)).
func (op *CosOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Compute sin(input)
	sinInput := backend.Sin(op.input)

	// Create tensor with -1
	negOne := createScalar(sinInput.Shape(), sinInput.DType(), -1.0, backend.Device())

	// -sin(input)
	negSin := backend.Mul(negOne, sinInput)

	// grad_input = grad_output * (-sin(input))
	gradInput := backend.Mul(outputGrad, negSin)

	return []*tensor.RawTensor{gradInput}
}

// Inputs returns the input tensor [x].
func (op *CosOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor cos(x).
func (op *CosOp) Output() *tensor.RawTensor {
	return op.output
}
