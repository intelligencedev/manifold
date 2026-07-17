package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Embedding is a lookup table that maps discrete indices to dense vectors.
//
// This is a fundamental layer in NLP and sequence models, converting token IDs
// to continuous embeddings. The embedding vectors are learnable parameters.
//
// Architecture:
//   - Weight: [NumEmbed, EmbedDim] learnable parameter
//   - Forward: indices [batch, seq] -> embeddings [batch, seq, EmbedDim]
//   - Backward: gradients scatter-add to weight rows
//
// Example:
//
//	// Vocabulary of 10000 words, embedding dimension 256
//	embed := nn.NewEmbedding[B](10000, 256, backend)
//
//	// Token IDs for batch of 2 sequences, each 5 tokens
//	indices := tensor.FromSlice([]int32{1, 2, 3, 4, 5, 10, 11, 12, 13, 14},
//	    tensor.Shape{2, 5}, backend)
//
//	// Get embeddings [2, 5, 256]
//	embeddings := embed.Forward(indices)
type Embedding[B tensor.Backend] struct {
	Weight   *Parameter[B] // Embedding weight matrix [NumEmbed, EmbedDim]
	NumEmbed int           // Number of embeddings (vocabulary size)
	EmbedDim int           // Embedding dimension (vector size)
}

// NewEmbedding creates a new Embedding layer.
//
// The embedding weights are initialized from a standard normal distribution N(0, 1).
// For other initialization strategies (Xavier, truncated normal), initialize the
// weight tensor manually and pass it to NewEmbeddingWithWeight.
//
// Parameters:
//   - numEmbeddings: Size of the embedding dictionary (e.g., vocabulary size)
//   - embeddingDim: Dimension of each embedding vector
//   - backend: Computation backend
//
// Returns a new Embedding layer with randomly initialized weights.
func NewEmbedding[B tensor.Backend](numEmbeddings, embeddingDim int, backend B) *Embedding[B] {
	// Initialize weight from N(0, 1)
	weightData := make([]float32, numEmbeddings*embeddingDim)
	for i := range weightData {
		weightData[i] = float32(randNormFloat64())
	}

	weight, err := tensor.FromSlice[float32, B](weightData, tensor.Shape{numEmbeddings, embeddingDim}, backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create embedding weight: %v", err))
	}

	return &Embedding[B]{
		Weight:   NewParameter[B]("embedding.weight", weight),
		NumEmbed: numEmbeddings,
		EmbedDim: embeddingDim,
	}
}

// NewEmbeddingWithWeight creates an Embedding layer with pre-initialized weights.
//
// Use this when you want custom initialization (Xavier, truncated normal, pretrained, etc.)
//
// Parameters:
//   - weight: Pre-initialized weight tensor [numEmbeddings, embeddingDim]
//
// Returns a new Embedding layer using the provided weights.
func NewEmbeddingWithWeight[B tensor.Backend](weight *tensor.Tensor[float32, B]) *Embedding[B] {
	shape := weight.Shape()
	if len(shape) != 2 {
		panic(fmt.Sprintf("embedding weight must be 2D, got shape %v", shape))
	}

	return &Embedding[B]{
		Weight:   NewParameter[B]("embedding.weight", weight),
		NumEmbed: shape[0],
		EmbedDim: shape[1],
	}
}

// Forward performs embedding lookup.
//
// Maps each index to its corresponding embedding vector.
// This operation is differentiable - gradients flow back to the weight tensor.
//
// Parameters:
//   - indices: Tensor of indices [batch, seq] or any shape [...] of type int32
//
// Returns:
//   - embeddings: Tensor [..., EmbedDim] with embedding vectors
//
// Example:
//
//	indices := tensor.FromSlice([]int32{0, 1, 2}, tensor.Shape{3}, backend)
//	embeddings := embed.Forward(indices) // Shape: [3, EmbedDim]
//
// Panics if any index is out of bounds [0, NumEmbed).
func (e *Embedding[B]) Forward(indices *tensor.Tensor[int32, B]) *tensor.Tensor[float32, B] {
	// Use tensor Embedding operation which records on autodiff tape
	return e.Weight.Tensor().Embedding(indices)
}

// Parameters returns the list of trainable parameters.
func (e *Embedding[B]) Parameters() []*Parameter[B] {
	return []*Parameter[B]{e.Weight}
}
