// Package nn provides neural network modules and layers for building deep learning models.
// It includes activations, attention mechanisms, normalization layers, and more.
package nn

import (
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// ScaledDotProductAttention computes attention scores using the scaled dot-product mechanism.
//
// This is the core attention mechanism used in transformers, implementing:
//
//	Attention(Q, K, V) = softmax(QK^T / sqrt(d_k)) * V
//
// Where:
//   - Q (query): what information we're looking for [batch, heads, seq_q, head_dim]
//   - K (key): what information is available [batch, heads, seq_k, head_dim]
//   - V (value): the actual information to retrieve [batch, heads, seq_k, head_dim]
//   - mask: optional attention mask (additive, -inf for masked positions)
//   - scale: scaling factor (typically 1/sqrt(head_dim)), 0 for auto-compute
//
// Parameters:
//   - query: Query tensor [batch, heads, seq_q, head_dim]
//   - key: Key tensor [batch, heads, seq_k, head_dim]
//   - value: Value tensor [batch, heads, seq_k, head_dim]
//   - mask: Optional attention mask [batch, 1, seq_q, seq_k] or nil (additive mask, -inf for masked)
//   - scale: Scaling factor (0 for auto-compute as 1/sqrt(head_dim))
//
// Returns:
//   - output: Attended values [batch, heads, seq_q, head_dim]
//   - weights: Attention weights [batch, heads, seq_q, seq_k]
//
// Example:
//
//	backend := autodiff.New(cpu.New())
//	Q := tensor.Randn[float32](tensor.Shape{2, 8, 10, 64}, backend)  // batch=2, heads=8, seq=10, dim=64
//	K := tensor.Randn[float32](tensor.Shape{2, 8, 10, 64}, backend)
//	V := tensor.Randn[float32](tensor.Shape{2, 8, 10, 64}, backend)
//	output, weights := nn.ScaledDotProductAttention(Q, K, V, nil, 0)  // auto-scale
func ScaledDotProductAttention[B tensor.Backend](
	query, key, value *tensor.Tensor[float32, B],
	mask *tensor.Tensor[float32, B],
	scale float32,
) (*tensor.Tensor[float32, B], *tensor.Tensor[float32, B]) {
	validateAttentionInputs(query, key, value)

	// Auto-compute scale if not provided
	qHeadDim := query.Shape()[3]
	if scale == 0 {
		scale = float32(1.0 / math.Sqrt(float64(qHeadDim)))
	}

	// 1. Compute attention scores: Q @ K^T using BatchMatMul
	// K^T: transpose last two dimensions [batch, heads, seq_k, head_dim] -> [batch, heads, head_dim, seq_k]
	kT := key.Transpose(0, 1, 3, 2)
	scores := query.BatchMatMul(kT)

	// 2. Scale
	scores = scores.MulScalar(scale)

	// 3. Apply mask (if provided)
	if mask != nil {
		scores = scores.Add(mask)
	}

	// 4. Softmax along last dimension (over keys)
	weights := scores.Softmax(-1)

	// 5. Compute output: weights @ V using BatchMatMul
	output := weights.BatchMatMul(value)

	return output, weights
}

// validateAttentionInputs validates the input tensors for attention.
func validateAttentionInputs[B tensor.Backend](
	query, key, value *tensor.Tensor[float32, B],
) {
	if len(query.Shape()) != 4 {
		panic("ScaledDotProductAttention: query must be 4D [batch, heads, seq_q, head_dim]")
	}
	if len(key.Shape()) != 4 {
		panic("ScaledDotProductAttention: key must be 4D [batch, heads, seq_k, head_dim]")
	}
	if len(value.Shape()) != 4 {
		panic("ScaledDotProductAttention: value must be 4D [batch, heads, seq_k, head_dim]")
	}

	// Q and K must have same head_dim
	qHeadDim := query.Shape()[3]
	kHeadDim := key.Shape()[3]
	if qHeadDim != kHeadDim {
		panic("ScaledDotProductAttention: query and key must have same head_dim")
	}

	// K and V must have same seq length
	kSeqLen := key.Shape()[2]
	vSeqLen := value.Shape()[2]
	if kSeqLen != vSeqLen {
		panic("ScaledDotProductAttention: key and value must have same seq length")
	}
}

// CausalMask creates a causal (autoregressive) attention mask.
//
// In causal attention, each position can only attend to earlier positions
// (including itself). This is used in autoregressive models like GPT.
//
// Returns a mask tensor where:
//   - Upper triangle (future positions) = -inf (masked out)
//   - Lower triangle + diagonal (past + current) = 0 (allowed)
//
// The mask is applied additively to attention scores before softmax:
//
//	scores = QK^T / sqrt(d_k) + mask
//
// Shape: [1, 1, seq_len, seq_len] (broadcastable to [batch, heads, seq, seq])
//
// Example:
//
//	// For seq_len=4:
//	// [[0,   -inf, -inf, -inf],
//	//  [0,   0,    -inf, -inf],
//	//  [0,   0,    0,    -inf],
//	//  [0,   0,    0,    0   ]]
//
//	backend := cpu.New()
//	mask := nn.CausalMask(10, backend)  // [1, 1, 10, 10]
func CausalMask[B tensor.Backend](seqLen int, backend B) *tensor.Tensor[float32, B] {
	// Create a mask of shape [1, 1, seq_len, seq_len]
	mask := tensor.Zeros[float32](tensor.Shape{1, 1, seqLen, seqLen}, backend)

	// Fill upper triangle with -inf
	negInf := float32(math.Inf(-1))
	data := mask.Data()

	for i := 0; i < seqLen; i++ {
		for j := i + 1; j < seqLen; j++ {
			// Index in flattened array: [0, 0, i, j]
			// For shape [1, 1, seq_len, seq_len]:
			// index = 0*1*seq_len*seq_len + 0*seq_len*seq_len + i*seq_len + j
			idx := i*seqLen + j
			data[idx] = negInf
		}
	}

	return mask
}

// StandardAttention computes attention the traditional way.
//
// Memory: O(N²) - materializes full attention matrix.
// This implementation is used for correctness validation of Flash Attention.
//
// Implements: Attention(Q, K, V) = softmax(QK^T / sqrt(d_k)) * V
//
// Parameters:
//   - q: Query tensor [batch * seqLen * numHeads * headDim] (flattened).
//   - k: Key tensor [batch * kvLen * numHeads * headDim] (flattened).
//   - v: Value tensor [batch * kvLen * numHeads * headDim] (flattened).
//   - batch: Batch size.
//   - seqLen: Query sequence length.
//   - kvLen: Key/Value sequence length.
//   - numHeads: Number of attention heads.
//   - headDim: Dimension of each head.
//   - scale: Scaling factor (typically 1/sqrt(headDim)).
//   - causal: Whether to apply causal masking.
//
// Returns:
//   - []float32: Output tensor [batch * seqLen * numHeads * headDim] (flattened).
//
// Example:
//
//	output := nn.StandardAttention(q, k, v, 2, 128, 128, 8, 64, 0.125, false)
//
//nolint:gocognit // High complexity is inherent to standard attention with nested loops for batch/head/query/key positions
func StandardAttention(
	q, k, v []float32,
	batch, seqLen, kvLen, numHeads, headDim int,
	scale float32,
	causal bool,
) []float32 {
	// Output: [batch, seqLen, numHeads, headDim]
	output := make([]float32, batch*seqLen*numHeads*headDim)

	// Process each batch and head independently
	for b := 0; b < batch; b++ {
		for h := 0; h < numHeads; h++ {
			// For each query position
			for i := 0; i < seqLen; i++ {
				// 1. Compute attention scores: Q @ K^T
				scores := make([]float32, kvLen)
				for j := 0; j < kvLen; j++ {
					score := float32(0)
					for d := 0; d < headDim; d++ {
						qIdx := b*seqLen*numHeads*headDim + i*numHeads*headDim + h*headDim + d
						kIdx := b*kvLen*numHeads*headDim + j*numHeads*headDim + h*headDim + d
						score += q[qIdx] * k[kIdx]
					}
					scores[j] = score * scale
				}

				// 2. Apply causal mask if needed
				if causal {
					for j := i + 1; j < kvLen; j++ {
						scores[j] = float32(math.Inf(-1))
					}
				}

				// 3. Softmax
				weights := attentionSoftmax(scores)

				// 4. Compute weighted sum of values
				for d := 0; d < headDim; d++ {
					sum := float32(0)
					for j := 0; j < kvLen; j++ {
						vIdx := b*kvLen*numHeads*headDim + j*numHeads*headDim + h*headDim + d
						sum += weights[j] * v[vIdx]
					}
					outIdx := b*seqLen*numHeads*headDim + i*numHeads*headDim + h*headDim + d
					output[outIdx] = sum
				}
			}
		}
	}

	return output
}

// softmax computes softmax over a 1D array.
func attentionSoftmax(x []float32) []float32 {
	// Find max for numerical stability
	maxVal := x[0]
	for _, val := range x[1:] {
		if val > maxVal {
			maxVal = val
		}
	}

	// Compute exp(x - max) and sum
	exp := make([]float32, len(x))
	sum := float32(0)
	for i, val := range x {
		exp[i] = float32(math.Exp(float64(val - maxVal)))
		sum += exp[i]
	}

	// Normalize
	result := make([]float32, len(x))
	for i := range result {
		result[i] = exp[i] / sum
	}
	return result
}
