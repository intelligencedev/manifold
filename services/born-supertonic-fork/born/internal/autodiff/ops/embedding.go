package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// EmbeddingOp represents an embedding lookup operation.
//
// Forward: output[i] = weight[indices[i]]
//
// Backward:
//
//	For each index i, accumulate grad_output[i] to grad_weight[indices[i]]
//	This is a scatter-add operation where gradients for the same index are summed.
//
// Example:
//
//	indices = [0, 1, 0]  // index 0 appears twice
//	grad_output = [[1,2], [3,4], [5,6]]
//	grad_weight[0] = [1,2] + [5,6] = [6,8]  // Accumulated!
//	grad_weight[1] = [3,4]
type EmbeddingOp struct {
	weight  *tensor.RawTensor // Embedding weight [numEmbeddings, embeddingDim]
	indices *tensor.RawTensor // Index tensor (int32)
	output  *tensor.RawTensor // Output embeddings
}

// NewEmbeddingOp creates a new embedding operation.
func NewEmbeddingOp(weight, indices, output *tensor.RawTensor) *EmbeddingOp {
	return &EmbeddingOp{
		weight:  weight,
		indices: indices,
		output:  output,
	}
}

// Inputs returns the input tensors (weight and indices).
// Note: Only weight needs gradient; indices are integer indices.
func (op *EmbeddingOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.weight}
}

// Output returns the output tensor.
func (op *EmbeddingOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes gradients for the embedding weights.
//
// Gradient computation:
//   - For each position i in output, grad_output[i] flows back to weight[indices[i]]
//   - Multiple indices pointing to the same embedding accumulate (scatter-add)
//
// Uses backend.SelectAdd to delegate the scatter-add so that GPU backends can
// accelerate it in the future without any change to this function.
func (op *EmbeddingOp) Backward(gradOutput *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	weightShape := op.weight.Shape()
	embDim := weightShape[1]

	// Flatten multi-dimensional inputs to 2D for SelectAdd.
	// gradOutput: [B, S, D] or [N, D] → [N, D]
	// indices:    [B, S] or [N]       → [N]
	numIndices := op.indices.NumElements()
	flatGrad := backend.Reshape(gradOutput, tensor.Shape{numIndices, embDim})
	flatIdx := backend.Reshape(op.indices, tensor.Shape{numIndices})

	// Zero-filled destination: same shape as the weight matrix [numEmbeddings, embDim].
	gradWeight, err := tensor.NewRaw(weightShape, gradOutput.DType(), backend.Device())
	if err != nil {
		panic(err)
	}

	// Scatter-add: for each i, gradWeight[flatIdx[i], :] += flatGrad[i, :]
	gradWeight = backend.SelectAdd(gradWeight, 0, flatIdx, flatGrad)

	return []*tensor.RawTensor{gradWeight}
}
