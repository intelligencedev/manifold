package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// MultiHeadAttention implements the multi-head attention mechanism.
//
// Architecture:
//
//	MHA(Q, K, V) = Concat(head_1, ..., head_h) * W_O
//	head_i = SDPA(Q*W_Q_i, K*W_K_i, V*W_V_i)
//
// This is the core attention layer used in all transformer architectures
// including BERT, GPT, LLaMA, and others.
//
// Example:
//
//	backend := autodiff.New(cpu.New())
//	mha := nn.NewMultiHeadAttention[B](768, 12, backend)  // 768 dim, 12 heads
//	output := mha.Forward(x, x, x, nil)  // Self-attention
//	output := mha.Forward(q, kv, kv, mask)  // Cross-attention
type MultiHeadAttention[B tensor.Backend] struct {
	WQ       *Linear[B] // Query projection [embed_dim, embed_dim]
	WK       *Linear[B] // Key projection [embed_dim, embed_dim]
	WV       *Linear[B] // Value projection [embed_dim, embed_dim]
	WO       *Linear[B] // Output projection [embed_dim, embed_dim]
	NumHeads int
	HeadDim  int
	EmbedDim int
	backend  B
}

// NewMultiHeadAttention creates a new multi-head attention module.
//
// Parameters:
//   - embedDim: Total embedding dimension (must be divisible by numHeads)
//   - numHeads: Number of attention heads
//   - backend: Computation backend
//
// The head dimension is computed as embedDim / numHeads.
//
// Example:
//
//	mha := nn.NewMultiHeadAttention[B](768, 12, backend)
//	// embedDim=768, numHeads=12 -> headDim=64
func NewMultiHeadAttention[B tensor.Backend](
	embedDim, numHeads int,
	backend B,
) *MultiHeadAttention[B] {
	if embedDim%numHeads != 0 {
		panic(fmt.Sprintf("MultiHeadAttention: embed_dim (%d) must be divisible by num_heads (%d)", embedDim, numHeads))
	}
	headDim := embedDim / numHeads

	return &MultiHeadAttention[B]{
		WQ:       NewLinear[B](embedDim, embedDim, backend),
		WK:       NewLinear[B](embedDim, embedDim, backend),
		WV:       NewLinear[B](embedDim, embedDim, backend),
		WO:       NewLinear[B](embedDim, embedDim, backend),
		NumHeads: numHeads,
		HeadDim:  headDim,
		EmbedDim: embedDim,
		backend:  backend,
	}
}

// Forward computes multi-head attention.
//
// Args:
//   - query: Query tensor [batch, seq_q, embed_dim]
//   - key: Key tensor [batch, seq_k, embed_dim]
//   - value: Value tensor [batch, seq_k, embed_dim]
//   - mask: Optional attention mask [batch, 1, seq_q, seq_k] or nil
//
// Returns:
//   - output: [batch, seq_q, embed_dim]
//
// For self-attention, pass the same tensor for query, key, and value.
// For cross-attention, query differs from key/value.
func (m *MultiHeadAttention[B]) Forward(
	query, key, value *tensor.Tensor[float32, B],
	mask *tensor.Tensor[float32, B],
) *tensor.Tensor[float32, B] {
	batch := query.Shape()[0]
	seqQ := query.Shape()[1]
	seqK := key.Shape()[1]

	// 1. Project Q, K, V through linear layers
	// Linear expects 2D input [batch*seq, embed_dim]
	q := m.projectAndReshape(query, m.WQ, batch, seqQ)
	k := m.projectAndReshape(key, m.WK, batch, seqK)
	v := m.projectAndReshape(value, m.WV, batch, seqK)

	// 2. Reshape to [batch, seq, num_heads, head_dim] then transpose to [batch, num_heads, seq, head_dim]
	q = q.Reshape(batch, seqQ, m.NumHeads, m.HeadDim).Transpose(0, 2, 1, 3)
	k = k.Reshape(batch, seqK, m.NumHeads, m.HeadDim).Transpose(0, 2, 1, 3)
	v = v.Reshape(batch, seqK, m.NumHeads, m.HeadDim).Transpose(0, 2, 1, 3)

	// 3. Scaled dot-product attention (uses BatchMatMul internally)
	attnOut, _ := ScaledDotProductAttention(q, k, v, mask, 0)

	// 4. Transpose back and reshape to [batch, seq_q, embed_dim]
	attnOut = attnOut.Transpose(0, 2, 1, 3).Reshape(batch, seqQ, m.EmbedDim)

	// 5. Output projection
	// Reshape for Linear, then reshape back
	attnOut2D := attnOut.Reshape(batch*seqQ, m.EmbedDim)
	output := m.WO.Forward(attnOut2D)
	output = output.Reshape(batch, seqQ, m.EmbedDim)

	return output
}

// ForwardWithWeights computes multi-head attention and returns attention weights.
//
// Same as Forward but also returns attention weights for visualization/analysis.
//
// Returns:
//   - output: [batch, seq_q, embed_dim]
//   - weights: [batch, num_heads, seq_q, seq_k]
func (m *MultiHeadAttention[B]) ForwardWithWeights(
	query, key, value *tensor.Tensor[float32, B],
	mask *tensor.Tensor[float32, B],
) (*tensor.Tensor[float32, B], *tensor.Tensor[float32, B]) {
	batch := query.Shape()[0]
	seqQ := query.Shape()[1]
	seqK := key.Shape()[1]

	// 1. Project Q, K, V
	q := m.projectAndReshape(query, m.WQ, batch, seqQ)
	k := m.projectAndReshape(key, m.WK, batch, seqK)
	v := m.projectAndReshape(value, m.WV, batch, seqK)

	// 2. Reshape for multi-head
	q = q.Reshape(batch, seqQ, m.NumHeads, m.HeadDim).Transpose(0, 2, 1, 3)
	k = k.Reshape(batch, seqK, m.NumHeads, m.HeadDim).Transpose(0, 2, 1, 3)
	v = v.Reshape(batch, seqK, m.NumHeads, m.HeadDim).Transpose(0, 2, 1, 3)

	// 3. SDPA with weights
	attnOut, weights := ScaledDotProductAttention(q, k, v, mask, 0)

	// 4. Reshape output
	attnOut = attnOut.Transpose(0, 2, 1, 3).Reshape(batch, seqQ, m.EmbedDim)

	// 5. Output projection
	attnOut2D := attnOut.Reshape(batch*seqQ, m.EmbedDim)
	output := m.WO.Forward(attnOut2D)
	output = output.Reshape(batch, seqQ, m.EmbedDim)

	return output, weights
}

// projectAndReshape is a helper that reshapes 3D input to 2D, applies Linear, and reshapes back to 3D.
func (m *MultiHeadAttention[B]) projectAndReshape(
	input *tensor.Tensor[float32, B],
	linear *Linear[B],
	batch, seq int,
) *tensor.Tensor[float32, B] {
	// Reshape [batch, seq, embed_dim] -> [batch*seq, embed_dim]
	input2D := input.Reshape(batch*seq, m.EmbedDim)

	// Apply linear projection
	output2D := linear.Forward(input2D)

	// Reshape back [batch*seq, embed_dim] -> [batch, seq, embed_dim]
	return output2D.Reshape(batch, seq, m.EmbedDim)
}

// ForwardWithCache computes attention using KV cache for efficient autoregressive generation.
//
// This method is optimized for inference where tokens are generated one at a time.
// Instead of recomputing K,V for all previous tokens, we cache them and only compute
// for the new token.
//
// Args:
//   - query: Query tensor [batch, 1, embed_dim] (typically single token)
//   - cache: KV cache storing previous key-value pairs
//
// Returns:
//   - output: [batch, 1, embed_dim]
//
// The cache is automatically updated with new K,V pairs.
//
// Example:
//
//	cache := nn.NewKVCache[B](1, 12, 512, 64, backend)
//	for i := 0; i < 100; i++ {
//	    token := getNextToken(i) // [1, 1, 768]
//	    output := mha.ForwardWithCache(token, cache)
//	}
func (m *MultiHeadAttention[B]) ForwardWithCache(
	query *tensor.Tensor[float32, B],
	cache *KVCache[B],
) *tensor.Tensor[float32, B] {
	batch := query.Shape()[0]
	seqQ := query.Shape()[1] // Typically 1 for autoregressive generation

	// 1. Project Q, K, V for the new token
	q := m.projectAndReshape(query, m.WQ, batch, seqQ)
	k := m.projectAndReshape(query, m.WK, batch, seqQ) // Use query as input for self-attention
	v := m.projectAndReshape(query, m.WV, batch, seqQ)

	// 2. Reshape to [batch, seq, num_heads, head_dim] then transpose to [batch, num_heads, seq, head_dim]
	q = q.Reshape(batch, seqQ, m.NumHeads, m.HeadDim).Transpose(0, 2, 1, 3)
	k = k.Reshape(batch, seqQ, m.NumHeads, m.HeadDim).Transpose(0, 2, 1, 3)
	v = v.Reshape(batch, seqQ, m.NumHeads, m.HeadDim).Transpose(0, 2, 1, 3)

	// 3. Update cache with new K,V
	cache.Update(k, v)

	// 4. Get all cached K,V (including the new one)
	cachedK, cachedV := cache.Get()

	// 5. Scaled dot-product attention with cached K,V
	// q: [batch, num_heads, 1, head_dim]
	// cachedK, cachedV: [batch, num_heads, cache_len, head_dim]
	attnOut, _ := ScaledDotProductAttention(q, cachedK, cachedV, nil, 0)

	// 6. Transpose back and reshape to [batch, seq_q, embed_dim]
	attnOut = attnOut.Transpose(0, 2, 1, 3).Reshape(batch, seqQ, m.EmbedDim)

	// 7. Output projection
	attnOut2D := attnOut.Reshape(batch*seqQ, m.EmbedDim)
	output := m.WO.Forward(attnOut2D)
	output = output.Reshape(batch, seqQ, m.EmbedDim)

	return output
}

// Parameters returns all trainable parameters (WQ, WK, WV, WO weights and biases).
func (m *MultiHeadAttention[B]) Parameters() []*Parameter[B] {
	params := make([]*Parameter[B], 0, 8)
	params = append(params, m.WQ.Parameters()...)
	params = append(params, m.WK.Parameters()...)
	params = append(params, m.WV.Parameters()...)
	params = append(params, m.WO.Parameters()...)
	return params
}
