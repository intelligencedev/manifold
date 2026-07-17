// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package nn

import (
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// TransformerConfig defines the configuration for a Transformer Block.
//
// Fields:
//   - EmbedDim: Embedding dimension (d_model, e.g., 768 for GPT-2)
//   - NumHeads: Number of attention heads (e.g., 12 for GPT-2)
//   - FFNDim: FFN hidden dimension (typically 4 * EmbedDim)
//   - Dropout: Dropout rate (0 = no dropout, not yet implemented)
//   - NormFirst: true = Pre-Norm (LLaMA), false = Post-Norm (original)
//   - UseRMSNorm: true = RMSNorm (LLaMA), false = LayerNorm (BERT/GPT)
//   - NormEps: Normalization epsilon (1e-5 typical)
//
// Example:
//
//	config := nn.TransformerConfig{
//	    EmbedDim:   768,
//	    NumHeads:   12,
//	    FFNDim:     3072,
//	    NormFirst:  true,
//	    UseRMSNorm: true,
//	    NormEps:    1e-5,
//	}
type TransformerConfig = nn.TransformerConfig

// TransformerBlock is a complete Transformer Block with attention and FFN.
//
// Architecture (Pre-Norm):
//
//	x → Norm → MHA → + → Norm → FFN → + → output
//	         ↑_______|         ↑_______|
//	       (residual)        (residual)
//
// Used in all transformer models (GPT, BERT, LLaMA, etc.)
type TransformerBlock[B tensor.Backend] = nn.TransformerBlock[B]

// NewTransformerBlock creates a new Transformer Block.
//
// Parameters:
//   - config: Configuration (embedDim, numHeads, ffnDim, etc.)
//   - backend: Computation backend
//
// Example:
//
//	backend := autodiff.New(cpu.New())
//	config := nn.TransformerConfig{
//	    EmbedDim:   768,
//	    NumHeads:   12,
//	    FFNDim:     3072,
//	    NormFirst:  true,
//	    UseRMSNorm: true,
//	    NormEps:    1e-5,
//	}
//	block := nn.NewTransformerBlock(config, backend)
//	output := block.Forward(x, mask)
func NewTransformerBlock[B tensor.Backend](config TransformerConfig, backend B) *TransformerBlock[B] {
	return nn.NewTransformerBlock(config, backend)
}

// FFN (Feed-Forward Network) is a 2-layer MLP with SiLU activation.
//
// Architecture:
//
//	FFN(x) = Linear2(SiLU(Linear1(x)))
//
// Used inside TransformerBlock.
type FFN[B tensor.Backend] = nn.FFN[B]

// NewFFN creates a new Feed-Forward Network.
//
// Parameters:
//   - embedDim: Input/output dimension
//   - ffnDim: Hidden dimension (typically 4 * embedDim)
//   - backend: Computation backend
//
// Example:
//
//	ffn := nn.NewFFN[B](768, 3072, backend)
//	output := ffn.Forward(x)
func NewFFN[B tensor.Backend](embedDim, ffnDim int, backend B) *FFN[B] {
	return nn.NewFFN(embedDim, ffnDim, backend)
}
