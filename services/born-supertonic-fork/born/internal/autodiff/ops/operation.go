// Package ops defines operation interfaces and implementations for automatic differentiation.
//
// Each operation implements the Operation interface, which provides:
//   - Forward pass: computed by the backend
//   - Backward pass: computes gradients for inputs given output gradient
//
// Supported operations:
//   - AddOp: element-wise addition (d(a+b)/da = 1, d(a+b)/db = 1)
//   - SubOp: element-wise subtraction
//   - MulOp: element-wise multiplication (d(a*b)/da = b, d(a*b)/db = a)
//   - DivOp: element-wise division
//   - MatMulOp: matrix multiplication (d(A@B)/dA = grad@B^T, d(A@B)/dB = A^T@grad)
//   - ReLUOp: rectified linear unit activation (d(ReLU(x))/dx = 1 if x > 0, else 0)
package ops

import "github.com/born-ml/born/internal/tensor"

// Operation represents a differentiable operation in the computation graph.
// Each operation records its inputs and output during the forward pass,
// and computes input gradients during the backward pass.
type Operation interface {
	// Backward computes gradients for inputs given the output gradient.
	// Returns a slice of gradients corresponding to each input tensor.
	//
	// Example for AddOp:
	//   inputs: [a, b]
	//   outputGrad: dL/d(a+b)
	//   returns: [dL/d(a+b), dL/d(a+b)] (gradient flows equally to both inputs)
	Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor

	// Inputs returns the input tensors for this operation.
	Inputs() []*tensor.RawTensor

	// Output returns the output tensor produced by this operation.
	Output() *tensor.RawTensor
}

// MultiOutputOperation represents an operation that produces multiple outputs.
// Examples: Chunk (splits tensor into multiple parts), Split.
//
// The tape handles these specially by collecting gradients for ALL outputs
// before calling BackwardMulti.
type MultiOutputOperation interface {
	Operation

	// Outputs returns all output tensors produced by this operation.
	Outputs() []*tensor.RawTensor

	// BackwardMulti computes gradients for inputs given gradients for ALL outputs.
	// This is used instead of Backward for multi-output operations.
	//
	// Example for ChunkOp (splits [a,b,c,d] into [a,b] and [c,d]):
	//   outputGrads: [grad_chunk1, grad_chunk2]
	//   returns: [grad_input] where grad_input = Cat(outputGrads)
	BackwardMulti(outputGrads []*tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor
}
