package nn

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// RotaryEncoding implements Rotary Position Embedding (RoPE).
//
// RoPE is a modern positional encoding used in LLaMA, Mistral, DeepSeek, and other
// state-of-the-art LLMs. It applies a rotation to query and key embeddings based on
// their position, allowing the model to capture relative position information.
//
// Mathematical formulation:
//
//	For position m and dimension pair (2i, 2i+1):
//	  θ_i = base^(-2i/d)  (typically base=10000)
//
//	  [q'_{2i}  ]   [cos(m·θ_i)  -sin(m·θ_i)] [q_{2i}  ]
//	  [q'_{2i+1}] = [sin(m·θ_i)   cos(m·θ_i)] [q_{2i+1}]
//
// Architecture:
//   - Pre-computes cos and sin values for all positions and dimensions
//   - Applies rotation by splitting input into even/odd pairs
//   - Supports both training (full sequence) and inference (with offset for KV-cache)
//
// Example:
//
//	config := nn.RotaryEncodingConfig{
//	    DModel:    64,     // Head dimension (typically 64-128)
//	    MaxSeqLen: 2048,   // Maximum sequence length
//	    Theta:     10000.0,
//	}
//	rope := nn.NewRotaryEncoding(config, backend)
//
//	// During training: apply to full sequence
//	q := tensor.Randn[float32](tensor.Shape{batch, heads, seq, 64}, backend)
//	q_rotated := rope.Forward(q)
//
//	// During inference with KV-cache: apply with position offset
//	q_new := tensor.Randn[float32](tensor.Shape{batch, heads, 1, 64}, backend)
//	q_rotated := rope.ForwardWithOffset(q_new, currentPosition)
type RotaryEncoding[B tensor.Backend] struct {
	FreqCos   *tensor.Tensor[float32, B] // [max_seq_len, d_model/2] - cosine values
	FreqSin   *tensor.Tensor[float32, B] // [max_seq_len, d_model/2] - sine values
	MaxSeqLen int                        // Maximum sequence length
	DModel    int                        // Model dimension (must be even)
	backend   B
}

// RotaryEncodingConfig configures a RotaryEncoding layer.
type RotaryEncodingConfig struct {
	DModel    int     // Dimension per head (typically 64-128, must be even)
	MaxSeqLen int     // Maximum sequence length (e.g., 2048, 4096)
	Theta     float64 // Base frequency for rotation (default: 10000.0)
}

// NewRotaryEncoding creates a new RotaryEncoding layer.
//
// Pre-computes cosine and sine values for all positions and dimension pairs.
//
// Parameters:
//   - cfg: Configuration for RoPE (dimension, max sequence length, theta base)
//   - backend: Computation backend
//
// Returns a new RotaryEncoding layer with pre-computed rotation matrices.
//
// Panics if DModel is not even (RoPE requires pairing dimensions).
func NewRotaryEncoding[B tensor.Backend](cfg RotaryEncodingConfig, backend B) *RotaryEncoding[B] {
	if cfg.DModel%2 != 0 {
		panic(fmt.Sprintf("RotaryEncoding: DModel must be even, got %d", cfg.DModel))
	}
	if cfg.MaxSeqLen <= 0 {
		panic(fmt.Sprintf("RotaryEncoding: MaxSeqLen must be positive, got %d", cfg.MaxSeqLen))
	}
	if cfg.Theta <= 0 {
		cfg.Theta = 10000.0 // Default theta
	}

	// Compute frequencies for each dimension pair
	// θ_i = base^(-2i/d) for i in [0, d/2)
	halfDim := cfg.DModel / 2
	freqs := make([]float32, halfDim)
	for i := 0; i < halfDim; i++ {
		// θ_i = theta^(-2i/d)
		exponent := -2.0 * float64(i) / float64(cfg.DModel)
		freqs[i] = float32(math.Pow(cfg.Theta, exponent))
	}

	// Pre-compute cos and sin for all positions
	cosData := make([]float32, cfg.MaxSeqLen*halfDim)
	sinData := make([]float32, cfg.MaxSeqLen*halfDim)

	for pos := 0; pos < cfg.MaxSeqLen; pos++ {
		for i := 0; i < halfDim; i++ {
			angle := float64(pos) * float64(freqs[i])
			idx := pos*halfDim + i
			cosData[idx] = float32(math.Cos(angle))
			sinData[idx] = float32(math.Sin(angle))
		}
	}

	// Create tensors
	freqCos, err := tensor.FromSlice[float32, B](cosData, tensor.Shape{cfg.MaxSeqLen, halfDim}, backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create cos tensor: %v", err))
	}

	freqSin, err := tensor.FromSlice[float32, B](sinData, tensor.Shape{cfg.MaxSeqLen, halfDim}, backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create sin tensor: %v", err))
	}

	return &RotaryEncoding[B]{
		FreqCos:   freqCos,
		FreqSin:   freqSin,
		MaxSeqLen: cfg.MaxSeqLen,
		DModel:    cfg.DModel,
		backend:   backend,
	}
}

// Forward applies rotary position embeddings to the input tensor.
//
// Supports both 3D and 4D input tensors:
//   - 3D: [batch, seq_len, d_model] - applies RoPE to entire sequence
//   - 4D: [batch, n_heads, seq_len, d_k] - applies RoPE per head (typical for attention)
//
// The rotation is applied to dimension pairs (2i, 2i+1) using pre-computed cos/sin values.
//
// Parameters:
//   - x: Input tensor [batch, seq_len, d_model] or [batch, n_heads, seq_len, d_k]
//
// Returns tensor with same shape as input, with rotary embeddings applied.
//
// Panics if sequence length exceeds MaxSeqLen or if last dimension doesn't match DModel.
func (r *RotaryEncoding[B]) Forward(x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	return r.ForwardWithOffset(x, 0)
}

// ForwardWithOffset applies rotary embeddings with a position offset.
//
// This is useful for incremental decoding with KV-cache, where new tokens are generated
// one at a time but need position embeddings that account for previous tokens.
//
// Parameters:
//   - x: Input tensor [batch, seq_len, d_model] or [batch, n_heads, seq_len, d_k]
//   - offset: Position offset (e.g., current position in KV-cache)
//
// Returns tensor with rotary embeddings applied at positions [offset, offset+seq_len).
//
// Example (KV-cache inference):
//
//	// Initial prompt: positions [0, prompt_len)
//	q_prompt := rope.Forward(q_prompt_tokens)
//
//	// Generate token 1: position [prompt_len]
//	q_new := rope.ForwardWithOffset(q_new_token, prompt_len)
//
//	// Generate token 2: position [prompt_len + 1]
//	q_new := rope.ForwardWithOffset(q_new_token, prompt_len + 1)
//
// Panics if offset + seq_len exceeds MaxSeqLen.
func (r *RotaryEncoding[B]) ForwardWithOffset(x *tensor.Tensor[float32, B], offset int) *tensor.Tensor[float32, B] {
	shape := x.Shape()
	var seqLen, dModel int

	// Determine shape format
	switch len(shape) {
	case 3:
		// [batch, seq_len, d_model]
		seqLen = shape[1]
		dModel = shape[2]
	case 4:
		// [batch, n_heads, seq_len, d_k]
		seqLen = shape[2]
		dModel = shape[3]
	default:
		panic(fmt.Sprintf("RotaryEncoding: input must be 3D or 4D, got shape %v", shape))
	}

	// Validate dimensions
	if dModel != r.DModel {
		panic(fmt.Sprintf("RotaryEncoding: expected last dimension %d, got %d", r.DModel, dModel))
	}
	if offset+seqLen > r.MaxSeqLen {
		panic(fmt.Sprintf("RotaryEncoding: offset + seq_len (%d) exceeds MaxSeqLen (%d)", offset+seqLen, r.MaxSeqLen))
	}

	// Extract cos/sin for the relevant positions [offset, offset+seqLen)
	// cos/sin shape: [max_seq_len, d_model/2]
	// Extract rows [offset:offset+seqLen] -> [seq_len, d_model/2]
	halfDim := r.DModel / 2
	cosData := r.FreqCos.Data()
	sinData := r.FreqSin.Data()

	posCosSinShape := tensor.Shape{seqLen, halfDim}

	// Extract cos for positions
	posCosData := make([]float32, seqLen*halfDim)
	posSinData := make([]float32, seqLen*halfDim)
	for pos := 0; pos < seqLen; pos++ {
		srcIdx := (offset + pos) * halfDim
		dstIdx := pos * halfDim
		copy(posCosData[dstIdx:dstIdx+halfDim], cosData[srcIdx:srcIdx+halfDim])
		copy(posSinData[dstIdx:dstIdx+halfDim], sinData[srcIdx:srcIdx+halfDim])
	}

	posCos, err := tensor.FromSlice[float32, B](posCosData, posCosSinShape, r.backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create position cos tensor: %v", err))
	}

	posSin, err := tensor.FromSlice[float32, B](posSinData, posCosSinShape, r.backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create position sin tensor: %v", err))
	}

	// Apply rotation to input
	return r.applyRotation(x, posCos, posSin)
}

// applyRotation applies the rotation matrix using cos/sin values.
//
// Rotation formula for dimension pair (2i, 2i+1):
//
//	x_rotated[2i]   = x[2i] * cos(θ) - x[2i+1] * sin(θ)
//	x_rotated[2i+1] = x[2i] * sin(θ) + x[2i+1] * cos(θ)
//
// This is implemented by:
//  1. Splitting input into even and odd indices
//  2. Applying rotation formula
//  3. Interleaving results back
func (r *RotaryEncoding[B]) applyRotation(
	x *tensor.Tensor[float32, B],
	posCos *tensor.Tensor[float32, B],
	posSin *tensor.Tensor[float32, B],
) *tensor.Tensor[float32, B] {
	shape := x.Shape()
	xData := x.Data()
	cosData := posCos.Data()
	sinData := posSin.Data()

	halfDim := r.DModel / 2
	var batchSize, numHeads, seqLen int
	is3D := len(shape) == 3

	if is3D {
		batchSize = shape[0]
		numHeads = 1
		seqLen = shape[1]
	} else {
		batchSize = shape[0]
		numHeads = shape[1]
		seqLen = shape[2]
	}

	// Create output
	outData := make([]float32, len(xData))

	// Apply rotation for each batch, head, and position
	for b := 0; b < batchSize; b++ {
		for h := 0; h < numHeads; h++ {
			for pos := 0; pos < seqLen; pos++ {
				// Base index for this (batch, head, position)
				var baseIdx int
				if is3D {
					baseIdx = b*seqLen*r.DModel + pos*r.DModel
				} else {
					baseIdx = b*numHeads*seqLen*r.DModel + h*seqLen*r.DModel + pos*r.DModel
				}

				// Index for cos/sin (only depends on position)
				cossinBaseIdx := pos * halfDim

				// Apply rotation using rotate-half convention (LLaMA/GPT-NeoX standard).
				// Pairs (x[i], x[i+d/2]) instead of interleaved (x[2i], x[2i+1]).
				for i := 0; i < halfDim; i++ {
					xi := xData[baseIdx+i]
					xiHalf := xData[baseIdx+halfDim+i]
					cosVal := cosData[cossinBaseIdx+i]
					sinVal := sinData[cossinBaseIdx+i]

					outData[baseIdx+i] = xi*cosVal - xiHalf*sinVal
					outData[baseIdx+halfDim+i] = xiHalf*cosVal + xi*sinVal
				}
			}
		}
	}

	// Create output tensor
	out, err := tensor.FromSlice[float32, B](outData, shape, r.backend)
	if err != nil {
		panic(fmt.Sprintf("failed to create output tensor: %v", err))
	}

	return out
}
