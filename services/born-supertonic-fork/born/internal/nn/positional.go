package nn

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// SinusoidalPositionalEncoding implements fixed sinusoidal positional encodings.
//
// This is the original positional encoding from "Attention is All You Need" (Vaswani et al., 2017).
// It uses sine and cosine functions at different frequencies to encode position information.
//
// Mathematical formulation:
//
//	PE(pos, 2i)   = sin(pos / 10000^(2i/d))
//	PE(pos, 2i+1) = cos(pos / 10000^(2i/d))
//
// Where:
//   - pos is the position (0 to max_len-1)
//   - i is the dimension (0 to d/2-1)
//   - d is the model dimension
//
// These encodings are fixed (not learned) and allow the model to easily learn to attend
// by relative positions, since for any fixed offset k, PE(pos+k) can be represented as
// a linear function of PE(pos).
//
// Example:
//
//	pe := nn.NewSinusoidalPositionalEncoding(512, 256, backend)
//	positions := pe.Forward(10)  // Get encodings for first 10 positions
//	// Shape: [1, 10, 256]
type SinusoidalPositionalEncoding[B tensor.Backend] struct {
	Encoding *tensor.Tensor[float32, B] // [max_len, dim] - pre-computed encodings
	MaxLen   int                        // Maximum sequence length
	Dim      int                        // Embedding dimension
	backend  B
}

// NewSinusoidalPositionalEncoding creates a new SinusoidalPositionalEncoding layer.
//
// Pre-computes all positional encodings up to maxLen.
//
// Parameters:
//   - maxLen: Maximum sequence length to pre-compute
//   - dim: Embedding dimension (typically same as model dimension)
//   - backend: Computation backend
//
// Returns a new SinusoidalPositionalEncoding with pre-computed encodings.
func NewSinusoidalPositionalEncoding[B tensor.Backend](maxLen, dim int, backend B) *SinusoidalPositionalEncoding[B] {
	if maxLen <= 0 {
		panic(fmt.Sprintf("SinusoidalPositionalEncoding: maxLen must be positive, got %d", maxLen))
	}
	if dim <= 0 {
		panic(fmt.Sprintf("SinusoidalPositionalEncoding: dim must be positive, got %d", dim))
	}

	// Pre-compute positional encodings
	encodings := make([]float32, maxLen*dim)

	for pos := 0; pos < maxLen; pos++ {
		for i := 0; i < dim; i++ {
			// Compute angle: pos / 10000^(2i/dim)
			angle := float64(pos) / math.Pow(10000.0, float64(2*(i/2))/float64(dim))

			idx := pos*dim + i
			if i%2 == 0 {
				// Even indices: sin
				encodings[idx] = float32(math.Sin(angle))
			} else {
				// Odd indices: cos
				encodings[idx] = float32(math.Cos(angle))
			}
		}
	}

	// Create tensor
	encoding, err := tensor.FromSlice[float32, B](encodings, tensor.Shape{maxLen, dim}, backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create encoding tensor: %v", err))
	}

	return &SinusoidalPositionalEncoding[B]{
		Encoding: encoding,
		MaxLen:   maxLen,
		Dim:      dim,
		backend:  backend,
	}
}

// Forward returns positional encodings for the specified sequence length.
//
// Parameters:
//   - seqLen: Length of the sequence (must be <= MaxLen)
//
// Returns:
//   - Positional encodings with shape [1, seqLen, dim]
//     The batch dimension is 1 for broadcasting to any batch size.
//
// Example:
//
//	pe := nn.NewSinusoidalPositionalEncoding(512, 256, backend)
//	encodings := pe.Forward(100)  // [1, 100, 256]
//
//	// Add to token embeddings
//	embeddings := tokenEmbed.Forward(tokens)  // [batch, 100, 256]
//	embeddings = embeddings.Add(encodings)    // Broadcast over batch
//
// Panics if seqLen > MaxLen.
func (s *SinusoidalPositionalEncoding[B]) Forward(seqLen int) *tensor.Tensor[float32, B] {
	if seqLen > s.MaxLen {
		panic(fmt.Sprintf("SinusoidalPositionalEncoding: seqLen %d exceeds MaxLen %d", seqLen, s.MaxLen))
	}

	// Extract first seqLen positions: [seqLen, dim]
	encData := s.Encoding.Data()
	seqData := make([]float32, seqLen*s.Dim)

	for pos := 0; pos < seqLen; pos++ {
		srcIdx := pos * s.Dim
		dstIdx := pos * s.Dim
		copy(seqData[dstIdx:dstIdx+s.Dim], encData[srcIdx:srcIdx+s.Dim])
	}

	seqEnc, err := tensor.FromSlice[float32, B](seqData, tensor.Shape{seqLen, s.Dim}, s.backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create sequence encoding: %v", err))
	}

	// Reshape to [1, seqLen, dim] for broadcasting
	return seqEnc.Reshape(1, seqLen, s.Dim)
}

// LearnedPositionalEmbedding implements learned positional embeddings.
//
// Unlike fixed sinusoidal encodings, these embeddings are learned parameters that
// are updated during training. This approach is used in GPT-2 and other models.
//
// Architecture:
//   - Embedding matrix: [MaxLen, Dim] - learned parameters
//   - Forward: returns embeddings for positions [0, seqLen)
//
// Example:
//
//	pe := nn.NewLearnedPositionalEmbedding(512, 256, backend)
//	positions := pe.Forward(100)  // Get learned embeddings for first 100 positions
//	// Shape: [1, 100, 256]
//
// The embeddings are initialized from a normal distribution N(0, 1).
type LearnedPositionalEmbedding[B tensor.Backend] struct {
	Embedding *Embedding[B] // Embedding layer for position indices
	MaxLen    int           // Maximum sequence length
	Dim       int           // Embedding dimension
	backend   B
}

// NewLearnedPositionalEmbedding creates a new LearnedPositionalEmbedding layer.
//
// The embeddings are initialized from a standard normal distribution N(0, 1).
//
// Parameters:
//   - maxLen: Maximum sequence length (number of position embeddings)
//   - dim: Embedding dimension (typically same as model dimension)
//   - backend: Computation backend
//
// Returns a new LearnedPositionalEmbedding with randomly initialized embeddings.
func NewLearnedPositionalEmbedding[B tensor.Backend](maxLen, dim int, backend B) *LearnedPositionalEmbedding[B] {
	if maxLen <= 0 {
		panic(fmt.Sprintf("LearnedPositionalEmbedding: maxLen must be positive, got %d", maxLen))
	}
	if dim <= 0 {
		panic(fmt.Sprintf("LearnedPositionalEmbedding: dim must be positive, got %d", dim))
	}

	// Create embedding layer for positions
	embedding := NewEmbedding[B](maxLen, dim, backend)

	return &LearnedPositionalEmbedding[B]{
		Embedding: embedding,
		MaxLen:    maxLen,
		Dim:       dim,
		backend:   backend,
	}
}

// Forward returns learned position embeddings for the specified sequence length.
//
// Parameters:
//   - seqLen: Length of the sequence (must be <= MaxLen)
//
// Returns:
//   - Position embeddings with shape [1, seqLen, dim]
//     The batch dimension is 1 for broadcasting to any batch size.
//
// Example:
//
//	pe := nn.NewLearnedPositionalEmbedding(512, 256, backend)
//	encodings := pe.Forward(100)  // [1, 100, 256]
//
//	// Add to token embeddings
//	embeddings := tokenEmbed.Forward(tokens)  // [batch, 100, 256]
//	embeddings = embeddings.Add(encodings)    // Broadcast over batch
//
// Panics if seqLen > MaxLen.
func (l *LearnedPositionalEmbedding[B]) Forward(seqLen int) *tensor.Tensor[float32, B] {
	if seqLen > l.MaxLen {
		panic(fmt.Sprintf("LearnedPositionalEmbedding: seqLen %d exceeds MaxLen %d", seqLen, l.MaxLen))
	}

	// Create position indices: [0, 1, 2, ..., seqLen-1]
	// seqLen is bounded by MaxLen (typically 2048-8192), safe for int32
	indices := make([]int32, seqLen)
	for i := 0; i < seqLen; i++ {
		indices[i] = int32(i)
	}

	indicesTensor, err := tensor.FromSlice[int32, B](indices, tensor.Shape{seqLen}, l.backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create indices tensor: %v", err))
	}

	// Get embeddings: [seqLen, dim]
	embeddings := l.Embedding.Forward(indicesTensor)

	// Reshape to [1, seqLen, dim] for broadcasting
	return embeddings.Reshape(1, seqLen, l.Dim)
}

// Parameters returns the trainable parameters (learned embeddings).
func (l *LearnedPositionalEmbedding[B]) Parameters() []*Parameter[B] {
	return l.Embedding.Parameters()
}

// ALiBi implements Attention with Linear Biases.
//
// ALiBi is a positional encoding method that adds a linear bias to attention scores
// based on the distance between query and key positions. This approach is used in
// BLOOM, MPT, and other models.
//
// Instead of adding positional information to embeddings, ALiBi adds a bias to the
// attention scores:
//
//	attention_scores = Q @ K^T + bias
//
// Where bias[i,j] = -slope * |i - j|, and each attention head has a different slope.
//
// The slopes are determined by a geometric sequence:
//
//	slopes = [2^(-8/n), 2^(-16/n), ..., 2^(-8)]  for n heads
//
// This allows the model to extrapolate to longer sequences than seen during training.
//
// Example:
//
//	alibi := nn.NewALiBi(8, backend)  // 8 attention heads
//	bias := alibi.GetBias(128)        // Bias for sequence length 128
//	// Shape: [1, 8, 128, 128]
//
//	// In attention:
//	scores := Q.BatchMatMul(K.Transpose())  // [batch, 8, seq, seq]
//	scores = scores.Add(bias)               // Add ALiBi bias
//	weights := scores.Softmax(-1)
type ALiBi[B tensor.Backend] struct {
	NumHeads int       // Number of attention heads
	Slopes   []float32 // Slope for each head (geometric sequence)
	backend  B
}

// NewALiBi creates a new ALiBi bias generator.
//
// Computes slopes for each attention head using the formula from the paper:
//
//	For n heads: slopes = [2^(-8/n * i) for i in 1..n]
//
// Example slopes for 8 heads:
//
//	[2^(-1), 2^(-2), 2^(-3), 2^(-4), 2^(-5), 2^(-6), 2^(-7), 2^(-8)]
//	≈ [0.5, 0.25, 0.125, 0.0625, 0.03125, 0.015625, 0.0078125, 0.00390625]
//
// Parameters:
//   - numHeads: Number of attention heads
//   - backend: Computation backend
//
// Returns a new ALiBi instance with pre-computed slopes.
func NewALiBi[B tensor.Backend](numHeads int, backend B) *ALiBi[B] {
	if numHeads <= 0 {
		panic(fmt.Sprintf("ALiBi: numHeads must be positive, got %d", numHeads))
	}

	// Compute slopes using geometric sequence
	slopes := make([]float32, numHeads)
	ratio := math.Pow(2, -8.0/float64(numHeads))

	for i := 0; i < numHeads; i++ {
		// slope_i = 2^(-8/n * (i+1))
		slopes[i] = float32(math.Pow(ratio, float64(i+1)))
	}

	return &ALiBi[B]{
		NumHeads: numHeads,
		Slopes:   slopes,
		backend:  backend,
	}
}

// GetBias returns the ALiBi bias matrix for the specified sequence length.
//
// The bias has shape [1, num_heads, seq_len, seq_len], where:
//   - bias[0, h, i, j] = -slopes[h] * |i - j|
//
// The leading dimension is 1 for broadcasting across batch dimension.
//
// Parameters:
//   - seqLen: Sequence length for the bias matrix
//
// Returns:
//   - Bias tensor [1, num_heads, seq_len, seq_len]
//
// Example:
//
//	alibi := nn.NewALiBi(8, backend)
//	bias := alibi.GetBias(64)  // [1, 8, 64, 64]
//
//	// In attention computation:
//	scores := Q.BatchMatMul(K.T())  // [batch, 8, 64, 64]
//	scores = scores.Add(bias)        // Broadcast and add
//	weights := scores.Softmax(-1)
func (a *ALiBi[B]) GetBias(seqLen int) *tensor.Tensor[float32, B] {
	if seqLen <= 0 {
		panic(fmt.Sprintf("ALiBi: seqLen must be positive, got %d", seqLen))
	}

	// Create bias tensor: [1, num_heads, seq_len, seq_len]
	totalSize := 1 * a.NumHeads * seqLen * seqLen
	biasData := make([]float32, totalSize)

	// Fill bias matrix
	for h := 0; h < a.NumHeads; h++ {
		slope := a.Slopes[h]
		for i := 0; i < seqLen; i++ {
			for j := 0; j < seqLen; j++ {
				// bias[i, j] = -slope * |i - j|
				distance := float32(abs(i - j))
				idx := h*seqLen*seqLen + i*seqLen + j
				biasData[idx] = -slope * distance
			}
		}
	}

	// Create tensor
	bias, err := tensor.FromSlice[float32, B](biasData, tensor.Shape{1, a.NumHeads, seqLen, seqLen}, a.backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create bias tensor: %v", err))
	}

	return bias
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
