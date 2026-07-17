package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// GroupedQueryAttention implements Grouped Query Attention (GQA).
//
// GQA is a variant of multi-head attention where the number of key-value heads
// is less than the number of query heads. This provides significant memory savings
// for KV-cache during inference while maintaining model quality.
//
// Architecture comparison:
//
//	MHA: n_q_heads = n_kv_heads (e.g., 32 Q, 32 K, 32 V)
//	GQA: n_q_heads > n_kv_heads (e.g., 32 Q, 8 K, 8 V) -> 4x memory savings
//	MQA: n_kv_heads = 1 (e.g., 32 Q, 1 K, 1 V) -> 32x memory savings (extreme)
//
// GQA is used in LLaMA 2/3, Mistral, DeepSeek, Qwen2, Phi-3, and other modern LLMs.
//
// Example:
//
//	cfg := nn.GQAConfig{
//	    EmbedDim:  4096,
//	    NQHeads:   32,
//	    NKVHeads:  8,    // 4:1 ratio
//	    HeadDim:   128,
//	    MaxSeqLen: 2048,
//	    UseRoPE:   true,
//	}
//	gqa := nn.NewGQA(cfg, backend)
//	output := gqa.Forward(x, cache, startPos)
type GroupedQueryAttention[B tensor.Backend] struct {
	QProj   *Linear[B] // Query projection [embed_dim, n_q_heads * head_dim]
	KProj   *Linear[B] // Key projection [embed_dim, n_kv_heads * head_dim]
	VProj   *Linear[B] // Value projection [embed_dim, n_kv_heads * head_dim]
	OutProj *Linear[B] // Output projection [n_q_heads * head_dim, embed_dim]

	rope    *RotaryEncoding[B] // Optional RoPE
	config  GQAConfig
	backend B
}

// GQAConfig configures a GroupedQueryAttention layer.
type GQAConfig struct {
	EmbedDim  int     // Model dimension (d_model)
	NQHeads   int     // Number of query heads
	NKVHeads  int     // Number of key-value heads (must divide NQHeads evenly)
	HeadDim   int     // Dimension per head
	Dropout   float32 // Dropout rate (not used in inference)
	UseRoPE   bool    // Whether to use Rotary Position Embeddings
	MaxSeqLen int     // Maximum sequence length (for RoPE)
	Theta     float64 // RoPE base frequency (default: 10000.0)
}

// NewGQA creates a new GroupedQueryAttention module.
//
// Validates that:
//   - NQHeads is divisible by NKVHeads
//   - EmbedDim equals NQHeads * HeadDim
//
// If HeadDim is 0, it's computed as EmbedDim / NQHeads.
//
// Example:
//
//	// LLaMA 2 7B style config
//	cfg := nn.GQAConfig{
//	    EmbedDim:  4096,
//	    NQHeads:   32,
//	    NKVHeads:  8,
//	    HeadDim:   128,
//	    UseRoPE:   true,
//	    MaxSeqLen: 4096,
//	}
//	gqa := nn.NewGQA(cfg, backend)
func NewGQA[B tensor.Backend](cfg GQAConfig, backend B) *GroupedQueryAttention[B] {
	// Validate config
	if cfg.NQHeads <= 0 {
		panic(fmt.Sprintf("GQA: NQHeads must be positive, got %d", cfg.NQHeads))
	}
	if cfg.NKVHeads <= 0 {
		panic(fmt.Sprintf("GQA: NKVHeads must be positive, got %d", cfg.NKVHeads))
	}
	if cfg.NQHeads%cfg.NKVHeads != 0 {
		panic(fmt.Sprintf("GQA: NQHeads (%d) must be divisible by NKVHeads (%d)",
			cfg.NQHeads, cfg.NKVHeads))
	}
	if cfg.EmbedDim <= 0 {
		panic(fmt.Sprintf("GQA: EmbedDim must be positive, got %d", cfg.EmbedDim))
	}

	// Compute HeadDim if not specified
	if cfg.HeadDim == 0 {
		cfg.HeadDim = cfg.EmbedDim / cfg.NQHeads
	}

	// Validate dimensions
	if cfg.NQHeads*cfg.HeadDim != cfg.EmbedDim {
		panic(fmt.Sprintf("GQA: NQHeads (%d) * HeadDim (%d) must equal EmbedDim (%d)",
			cfg.NQHeads, cfg.HeadDim, cfg.EmbedDim))
	}

	// Create projections
	qOutDim := cfg.NQHeads * cfg.HeadDim
	kvOutDim := cfg.NKVHeads * cfg.HeadDim

	gqa := &GroupedQueryAttention[B]{
		QProj:   NewLinear[B](cfg.EmbedDim, qOutDim, backend),
		KProj:   NewLinear[B](cfg.EmbedDim, kvOutDim, backend),
		VProj:   NewLinear[B](cfg.EmbedDim, kvOutDim, backend),
		OutProj: NewLinear[B](qOutDim, cfg.EmbedDim, backend),
		config:  cfg,
		backend: backend,
	}

	// Create RoPE if requested
	if cfg.UseRoPE {
		if cfg.MaxSeqLen <= 0 {
			cfg.MaxSeqLen = 2048 // Default max seq len
		}
		if cfg.Theta <= 0 {
			cfg.Theta = 10000.0
		}
		gqa.rope = NewRotaryEncoding[B](RotaryEncodingConfig{
			DModel:    cfg.HeadDim,
			MaxSeqLen: cfg.MaxSeqLen,
			Theta:     cfg.Theta,
		}, backend)
	}

	return gqa
}

// Forward computes grouped query attention with optional KV-cache.
//
// Args:
//   - x: Input tensor [batch, seq_len, embed_dim]
//   - cache: Optional KV-cache for efficient autoregressive generation
//   - startPos: Position offset for RoPE (used with KV-cache)
//
// Returns:
//   - Output tensor [batch, seq_len, embed_dim]
//
// The method automatically applies:
//   - RoPE to Q and K if configured
//   - KV head broadcasting (repeatKV) to match Q heads
//   - Causal masking for autoregressive attention
//
// Example:
//
//	// Training: process full sequence
//	output := gqa.Forward(x, nil, 0)
//
//	// Inference with KV-cache
//	cache := nn.NewKVCache[B](1, 8, 512, 128, backend)
//	output := gqa.Forward(x, cache, 0)  // First token(s)
//	output := gqa.Forward(nextToken, cache, seqLen)  // Subsequent tokens
func (g *GroupedQueryAttention[B]) Forward(
	x *tensor.Tensor[float32, B],
	cache *KVCache[B],
	startPos int,
) *tensor.Tensor[float32, B] {
	return g.ForwardWithMask(x, nil, cache, startPos)
}

// ForwardWithMask computes attention with a custom mask.
//
// Args:
//   - x: Input tensor [batch, seq_len, embed_dim]
//   - mask: Optional attention mask [batch, 1, seq_q, seq_k] or nil for auto causal mask
//   - cache: Optional KV-cache
//   - startPos: Position offset for RoPE
//
// Returns:
//   - Output tensor [batch, seq_len, embed_dim]
func (g *GroupedQueryAttention[B]) ForwardWithMask(
	x *tensor.Tensor[float32, B],
	mask *tensor.Tensor[float32, B],
	cache *KVCache[B],
	startPos int,
) *tensor.Tensor[float32, B] {
	batch := x.Shape()[0]
	seqLen := x.Shape()[1]

	// 1. Project Q, K, V
	q := g.project(x, g.QProj, batch, seqLen) // [batch, seq, n_q * head_dim]
	k := g.project(x, g.KProj, batch, seqLen) // [batch, seq, n_kv * head_dim]
	v := g.project(x, g.VProj, batch, seqLen) // [batch, seq, n_kv * head_dim]

	// 2. Reshape to [batch, seq, n_heads, head_dim]
	q = q.Reshape(batch, seqLen, g.config.NQHeads, g.config.HeadDim)
	k = k.Reshape(batch, seqLen, g.config.NKVHeads, g.config.HeadDim)
	v = v.Reshape(batch, seqLen, g.config.NKVHeads, g.config.HeadDim)

	// 3. Transpose to [batch, n_heads, seq, head_dim]
	q = q.Transpose(0, 2, 1, 3)
	k = k.Transpose(0, 2, 1, 3)
	v = v.Transpose(0, 2, 1, 3)

	// 4. Apply RoPE
	if g.rope != nil {
		q = g.rope.ForwardWithOffset(q, startPos)
		k = g.rope.ForwardWithOffset(k, startPos)
	}

	// 5. Update KV-cache if provided
	if cache != nil {
		cache.Update(k, v)
		k, v = cache.Get()
	}

	// 6. Repeat KV heads to match Q heads
	nRep := g.config.NQHeads / g.config.NKVHeads
	k = RepeatKV(k, nRep) // [batch, n_q, seq_k, head_dim]
	v = RepeatKV(v, nRep) // [batch, n_q, seq_k, head_dim]

	// 7. Create causal mask if not provided
	if mask == nil && cache != nil {
		// For cached inference, create mask for new query positions attending to all cached positions
		seqK := k.Shape()[2]
		mask = g.createCausalMask(seqLen, seqK, startPos)
	} else if mask == nil && seqLen > 1 {
		// For training/non-cached inference with multiple tokens
		mask = CausalMask(seqLen, g.backend)
	}

	// 8. Scaled dot-product attention
	output, _ := ScaledDotProductAttention(q, k, v, mask, 0)

	// 9. Transpose back to [batch, seq, n_q, head_dim]
	output = output.Transpose(0, 2, 1, 3)

	// 10. Reshape to [batch, seq, embed_dim]
	output = output.Reshape(batch, seqLen, g.config.EmbedDim)

	// 11. Output projection
	return g.project(output, g.OutProj, batch, seqLen)
}

// project applies a linear projection with reshape for 3D input.
func (g *GroupedQueryAttention[B]) project(
	x *tensor.Tensor[float32, B],
	linear *Linear[B],
	batch, seq int,
) *tensor.Tensor[float32, B] {
	inDim := x.Shape()[2]
	// Reshape to 2D: [batch*seq, dim]
	x2D := x.Reshape(batch*seq, inDim)
	// Apply linear
	out2D := linear.Forward(x2D)
	// Reshape to 3D: [batch, seq, out_dim]
	outDim := out2D.Shape()[1]
	return out2D.Reshape(batch, seq, outDim)
}

// createCausalMask creates a causal mask for incremental decoding.
// For a new query at position startPos, it can attend to all positions [0, startPos+seqLen).
func (g *GroupedQueryAttention[B]) createCausalMask(seqQ, seqK, _ /* startPos */ int) *tensor.Tensor[float32, B] {
	// For incremental generation, typically seqQ=1 (single new token)
	// and we need to allow attending to all previous positions
	if seqQ == 1 {
		// Single token can attend to all cached positions - no mask needed
		return nil
	}

	// For multiple query tokens, create proper causal mask
	return CausalMask(seqK, g.backend)
}

// Parameters returns all trainable parameters.
func (g *GroupedQueryAttention[B]) Parameters() []*Parameter[B] {
	params := make([]*Parameter[B], 0, 8)
	params = append(params, g.QProj.Parameters()...)
	params = append(params, g.KProj.Parameters()...)
	params = append(params, g.VProj.Parameters()...)
	params = append(params, g.OutProj.Parameters()...)
	return params
}

// RepeatKV broadcasts KV heads to match query heads count.
//
// This is the key operation in GQA that allows fewer KV heads than Q heads.
// Each KV head is repeated nRep times to match the Q head count.
//
// Input:  [batch, n_kv_heads, seq_len, head_dim]
// Output: [batch, n_q_heads, seq_len, head_dim] where n_q_heads = n_kv_heads * nRep
//
// Example:
//
//	// 8 KV heads -> 32 Q heads (nRep=4)
//	kv := tensor.Randn[float32](tensor.Shape{2, 8, 100, 128}, backend)
//	expanded := nn.RepeatKV(kv, 4)  // [2, 32, 100, 128]
//
// If nRep=1 (standard MHA), returns the input unchanged.
func RepeatKV[B tensor.Backend](
	kv *tensor.Tensor[float32, B],
	nRep int,
) *tensor.Tensor[float32, B] {
	if nRep == 1 {
		// MHA case: no repeat needed
		return kv
	}

	shape := kv.Shape()
	if len(shape) != 4 {
		panic(fmt.Sprintf("RepeatKV: expected 4D tensor [batch, n_kv, seq, head_dim], got shape %v", shape))
	}

	batch := shape[0]
	nKV := shape[1]
	seqLen := shape[2]
	headDim := shape[3]

	// Expand [batch, n_kv, seq, head_dim] -> [batch, n_kv, nRep, seq, head_dim]
	// Then reshape to [batch, n_kv*nRep, seq, head_dim]
	kvData := kv.Data()
	outData := make([]float32, batch*nKV*nRep*seqLen*headDim)

	// Copy each KV head nRep times
	for b := 0; b < batch; b++ {
		for h := 0; h < nKV; h++ {
			for r := 0; r < nRep; r++ {
				for s := 0; s < seqLen; s++ {
					// Source index: [b, h, s, :]
					srcBase := b*nKV*seqLen*headDim + h*seqLen*headDim + s*headDim
					// Destination index: [b, h*nRep+r, s, :]
					dstBase := b*nKV*nRep*seqLen*headDim + (h*nRep+r)*seqLen*headDim + s*headDim
					copy(outData[dstBase:dstBase+headDim], kvData[srcBase:srcBase+headDim])
				}
			}
		}
	}

	result, err := tensor.FromSlice[float32, B](outData, tensor.Shape{batch, nKV * nRep, seqLen, headDim}, kv.Backend())
	if err != nil {
		panic(fmt.Sprintf("RepeatKV: failed to create output tensor: %v", err))
	}

	return result
}

// MQA creates a Multi-Query Attention config (GQA with n_kv_heads=1).
//
// MQA is the extreme case of GQA where all query heads share a single KV head.
// This provides maximum memory savings but may reduce model capacity.
//
// Example:
//
//	cfg := nn.MQA(4096, 32, 128)  // 32 Q heads, 1 KV head
//	mqa := nn.NewGQA(cfg, backend)
func MQA(embedDim, nQHeads, headDim int) GQAConfig {
	return GQAConfig{
		EmbedDim: embedDim,
		NQHeads:  nQHeads,
		NKVHeads: 1, // Single KV head
		HeadDim:  headDim,
	}
}
