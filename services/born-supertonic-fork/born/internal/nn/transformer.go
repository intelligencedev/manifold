package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// TransformerConfig defines the configuration for a Transformer Block.
type TransformerConfig struct {
	EmbedDim   int     // d_model: Embedding dimension (e.g., 768 for GPT-2)
	NumHeads   int     // Number of attention heads (e.g., 12 for GPT-2)
	FFNDim     int     // FFN hidden dimension (typically 4 * EmbedDim = 3072)
	Dropout    float32 // Dropout rate (0 = no dropout, not implemented yet)
	NormFirst  bool    // true = Pre-Norm (LLaMA), false = Post-Norm (original Transformer)
	UseRMSNorm bool    // true = RMSNorm (LLaMA), false = LayerNorm (BERT/GPT)
	NormEps    float32 // Normalization epsilon (1e-5 typical, 1e-6 for RMSNorm)
}

// Normalizer is an interface for normalization layers (LayerNorm and RMSNorm).
//
// This allows TransformerBlock to work with both LayerNorm and RMSNorm
// without caring about the implementation details.
type Normalizer[B tensor.Backend] interface {
	Forward(x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B]
	Parameters() []*Parameter[B]
}

// TransformerBlock implements a complete Transformer Block.
//
// Architecture (Pre-Norm, LLaMA style):
//
//	x → LayerNorm → MHA → + → LayerNorm → FFN → + → output
//	         ↑_______|            ↑_______|
//	       (residual)           (residual)
//
// Architecture (Post-Norm, original Transformer):
//
//	x → MHA → + → LayerNorm → FFN → + → LayerNorm → output
//	     ↑___|                 ↑___|
//	   (residual)            (residual)
//
// Pre-Norm is preferred in modern LLMs as it provides:
//   - Better gradient flow (no need for learning rate warmup)
//   - More stable training
//   - Easier to stack many layers (100+ layers possible)
//
// Components:
//   - AttnNorm: Normalization before/after attention (RMSNorm or LayerNorm)
//   - Attention: Multi-Head Self-Attention (see MultiHeadAttention)
//   - FFNNorm: Normalization before/after FFN
//   - FFN: Feed-Forward Network (2-layer MLP with SiLU activation)
//
// Example:
//
//	backend := autodiff.New(cpu.New())
//	config := nn.TransformerConfig{
//	    EmbedDim:   768,
//	    NumHeads:   12,
//	    FFNDim:     3072,
//	    NormFirst:  true,   // Pre-Norm
//	    UseRMSNorm: true,   // RMSNorm
//	    NormEps:    1e-5,
//	}
//	block := nn.NewTransformerBlock(config, backend)
//	output := block.Forward(x, mask)  // [batch, seq, 768] -> [batch, seq, 768]
type TransformerBlock[B tensor.Backend] struct {
	Config    TransformerConfig
	AttnNorm  Normalizer[B] // RMSNorm or LayerNorm before/after attention
	Attention *MultiHeadAttention[B]
	FFNNorm   Normalizer[B] // RMSNorm or LayerNorm before/after FFN
	FFN       *FFN[B]
	backend   B
}

// NewTransformerBlock creates a new Transformer Block.
//
// Parameters:
//   - config: Configuration (embedDim, numHeads, ffnDim, normalization type, etc.)
//   - backend: Computation backend
//
// The block is initialized with:
//   - Multi-Head Attention (embedDim, numHeads)
//   - FFN (embedDim, ffnDim)
//   - Two normalization layers (RMSNorm or LayerNorm based on config)
//
// Example:
//
//	config := nn.TransformerConfig{
//	    EmbedDim:   768,
//	    NumHeads:   12,
//	    FFNDim:     3072,
//	    NormFirst:  true,   // Pre-Norm (LLaMA style)
//	    UseRMSNorm: true,   // RMSNorm (faster than LayerNorm)
//	    NormEps:    1e-5,
//	}
//	block := nn.NewTransformerBlock(config, backend)
func NewTransformerBlock[B tensor.Backend](config TransformerConfig, backend B) *TransformerBlock[B] {
	// Validate config
	if config.EmbedDim <= 0 {
		panic(fmt.Sprintf("TransformerBlock: embedDim must be positive, got %d", config.EmbedDim))
	}
	if config.NumHeads <= 0 {
		panic(fmt.Sprintf("TransformerBlock: numHeads must be positive, got %d", config.NumHeads))
	}
	if config.EmbedDim%config.NumHeads != 0 {
		panic(fmt.Sprintf("TransformerBlock: embedDim (%d) must be divisible by numHeads (%d)",
			config.EmbedDim, config.NumHeads))
	}
	if config.FFNDim <= 0 {
		panic(fmt.Sprintf("TransformerBlock: ffnDim must be positive, got %d", config.FFNDim))
	}
	if config.NormEps <= 0 {
		panic(fmt.Sprintf("TransformerBlock: normEps must be positive, got %f", config.NormEps))
	}

	// Create normalization layers (2 instances)
	var attnNorm, ffnNorm Normalizer[B]
	if config.UseRMSNorm {
		attnNorm = NewRMSNorm[B](config.EmbedDim, config.NormEps, backend)
		ffnNorm = NewRMSNorm[B](config.EmbedDim, config.NormEps, backend)
	} else {
		attnNorm = NewLayerNorm[B](config.EmbedDim, config.NormEps, backend)
		ffnNorm = NewLayerNorm[B](config.EmbedDim, config.NormEps, backend)
	}

	return &TransformerBlock[B]{
		Config:    config,
		AttnNorm:  attnNorm,
		Attention: NewMultiHeadAttention[B](config.EmbedDim, config.NumHeads, backend),
		FFNNorm:   ffnNorm,
		FFN:       NewFFN[B](config.EmbedDim, config.FFNDim, backend),
		backend:   backend,
	}
}

// Forward computes the transformer block output.
//
// Args:
//   - x: Input tensor [batch, seq, embed_dim]
//   - mask: Optional attention mask [batch, 1, seq, seq] or nil
//
// Returns:
//   - output: [batch, seq, embed_dim]
//
// The forward pass applies:
//  1. Self-Attention with residual connection
//  2. FFN with residual connection
//
// Normalization is applied either before (Pre-Norm) or after (Post-Norm) each sub-layer.
//
// Example:
//
//	x := tensor.Randn[float32](tensor.Shape{2, 16, 768}, backend)
//	mask := createCausalMask(16, backend)  // For autoregressive generation
//	output := block.Forward(x, mask)  // [2, 16, 768]
func (t *TransformerBlock[B]) Forward(x, mask *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	if t.Config.NormFirst {
		return t.forwardPreNorm(x, mask)
	}
	return t.forwardPostNorm(x, mask)
}

// forwardPreNorm implements Pre-Norm architecture (LLaMA style).
//
// x → Norm → MHA → + → Norm → FFN → + → output
//
//	  ↑_______|         ↑_______|
//	(residual)        (residual)
func (t *TransformerBlock[B]) forwardPreNorm(x, mask *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// 1. Attention block with residual
	// Norm -> MHA -> Add residual
	normed := t.AttnNorm.Forward(x)
	attnOut := t.Attention.Forward(normed, normed, normed, mask)
	x = x.Add(attnOut)

	// 2. FFN block with residual
	// Norm -> FFN -> Add residual
	normed = t.FFNNorm.Forward(x)
	ffnOut := t.FFN.Forward(normed)
	x = x.Add(ffnOut)

	return x
}

// forwardPostNorm implements Post-Norm architecture (original Transformer).
//
// x → MHA → + → Norm → FFN → + → Norm → output
//
//	  ↑___|              ↑___|
//	(residual)         (residual)
func (t *TransformerBlock[B]) forwardPostNorm(x, mask *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// 1. Attention block with residual
	// MHA -> Add residual -> Norm
	attnOut := t.Attention.Forward(x, x, x, mask)
	x = x.Add(attnOut)
	x = t.AttnNorm.Forward(x)

	// 2. FFN block with residual
	// FFN -> Add residual -> Norm
	ffnOut := t.FFN.Forward(x)
	x = x.Add(ffnOut)
	x = t.FFNNorm.Forward(x)

	return x
}

// ForwardWithCache computes attention using KV cache for efficient autoregressive generation.
//
// This method is optimized for inference where tokens are generated one at a time.
// The cache stores previous key-value pairs, avoiding recomputation.
//
// Args:
//   - x: Query tensor [batch, 1, embed_dim] (typically single token)
//   - cache: KV cache storing previous key-value pairs
//
// Returns:
//   - output: [batch, 1, embed_dim]
//
// Note: Only Pre-Norm is supported with cache. Post-Norm would require caching
// intermediate states which is more complex.
//
// Example:
//
//	cache := nn.NewKVCache[B](1, 12, 512, 64, backend)
//	for i := 0; i < 100; i++ {
//	    token := getNextToken(i) // [1, 1, 768]
//	    output := block.ForwardWithCache(token, cache)
//	}
func (t *TransformerBlock[B]) ForwardWithCache(
	x *tensor.Tensor[float32, B],
	cache *KVCache[B],
) *tensor.Tensor[float32, B] {
	// Only Pre-Norm is supported with cache
	if !t.Config.NormFirst {
		panic("TransformerBlock.ForwardWithCache: only Pre-Norm is supported with cache")
	}

	// 1. Attention block with residual and cache
	normed := t.AttnNorm.Forward(x)
	attnOut := t.Attention.ForwardWithCache(normed, cache)
	x = x.Add(attnOut)

	// 2. FFN block with residual
	normed = t.FFNNorm.Forward(x)
	ffnOut := t.FFN.Forward(normed)
	x = x.Add(ffnOut)

	return x
}

// Parameters returns all trainable parameters.
//
// Returns parameters from:
//   - AttnNorm (gamma, beta or just gamma for RMSNorm)
//   - Attention (WQ, WK, WV, WO weights and biases)
//   - FFNNorm (gamma, beta or just gamma for RMSNorm)
//   - FFN (Linear1, Linear2 weights and biases)
//
// Total parameters for GPT-2 768d/12h:
//   - Attention: ~2.4M params (4 * 768*768)
//   - AttnNorm: 768 (RMSNorm) or 1536 (LayerNorm)
//   - FFN: ~4.7M params (768*3072 + 3072*768)
//   - FFNNorm: 768 (RMSNorm) or 1536 (LayerNorm)
//   - Total: ~7.1M per block
func (t *TransformerBlock[B]) Parameters() []*Parameter[B] {
	params := make([]*Parameter[B], 0, 12) // Pre-allocate for efficiency
	params = append(params, t.AttnNorm.Parameters()...)
	params = append(params, t.Attention.Parameters()...)
	params = append(params, t.FFNNorm.Parameters()...)
	params = append(params, t.FFN.Parameters()...)
	return params
}
