package ops

import "github.com/born-ml/born/internal/tensor"

// MulOp represents an element-wise multiplication operation: output = a * b.
//
// Backward pass:
//   - d(a*b)/da = b, so grad_a = outputGrad * b
//   - d(a*b)/db = a, so grad_b = outputGrad * a
type MulOp struct {
	inputs []*tensor.RawTensor // [a, b]
	output *tensor.RawTensor   // a * b
}

// NewMulOp creates a new MulOp.
func NewMulOp(a, b, output *tensor.RawTensor) *MulOp {
	return &MulOp{
		inputs: []*tensor.RawTensor{a, b},
		output: output,
	}
}

// Backward computes input gradients for multiplication.
func (op *MulOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	a, b := op.inputs[0], op.inputs[1]

	// DEBUG: Print values
	// fmt.Printf("[MulOp.Backward] outputGrad=%v, a=%v, b=%v\n",
	//     outputGrad.AsFloat32(), a.AsFloat32(), b.AsFloat32())

	// grad_a = outputGrad * b
	gradA := backend.Mul(outputGrad, b)
	gradA = reduceBroadcast(gradA, a.Shape(), backend)

	// grad_b = outputGrad * a
	gradB := backend.Mul(outputGrad, a)
	gradB = reduceBroadcast(gradB, b.Shape(), backend)

	// DEBUG: Print results
	// fmt.Printf("[MulOp.Backward] gradA=%v, gradB=%v\n",
	//     gradA.AsFloat32(), gradB.AsFloat32())

	return []*tensor.RawTensor{gradA, gradB}
}

// Inputs returns the input tensors [a, b].
func (op *MulOp) Inputs() []*tensor.RawTensor {
	return op.inputs
}

// Output returns the output tensor a * b.
func (op *MulOp) Output() *tensor.RawTensor {
	return op.output
}
