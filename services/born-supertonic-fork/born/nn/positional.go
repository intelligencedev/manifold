// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

// Package nn provides public wrappers for positional encodings.
package nn

import (
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// Positional Encodings for Transformers

// RotaryEncoding implements Rotary Position Embedding (RoPE).
//
// RoPE is used in modern LLMs like LLaMA, Mistral, DeepSeek, and Qwen.
// It applies a rotation to query and key embeddings based on their position.
//
// Example:
//
//	backend := cpu.New()
//	config := nn.RotaryEncodingConfig{
//	    DModel:    64,
//	    MaxSeqLen: 2048,
//	    Theta:     10000.0,
//	}
//	rope := nn.NewRotaryEncoding(config, backend)
//
//	// Apply to attention queries/keys
//	q := tensor.Randn[float32](tensor.Shape{batch, heads, seq, 64}, backend)
//	q_rotated := rope.Forward(q)
type RotaryEncoding[B tensor.Backend] = nn.RotaryEncoding[B]

// RotaryEncodingConfig configures a RotaryEncoding layer.
type RotaryEncodingConfig = nn.RotaryEncodingConfig

// NewRotaryEncoding creates a new RoPE (Rotary Position Embedding) layer.
//
// Pre-computes cosine and sine values for all positions and dimension pairs.
//
// Parameters:
//   - cfg: Configuration (DModel, MaxSeqLen, Theta)
//   - backend: Computation backend
//
// Example:
//
//	config := nn.RotaryEncodingConfig{
//	    DModel:    64,     // Head dimension
//	    MaxSeqLen: 2048,   // Max sequence length
//	    Theta:     10000.0,
//	}
//	rope := nn.NewRotaryEncoding(config, backend)
func NewRotaryEncoding[B tensor.Backend](cfg RotaryEncodingConfig, backend B) *RotaryEncoding[B] {
	return nn.NewRotaryEncoding(cfg, backend)
}

// SinusoidalPositionalEncoding implements fixed sinusoidal positional encodings.
//
// This is the original positional encoding from "Attention is All You Need" (Vaswani et al., 2017).
//
// Example:
//
//	backend := cpu.New()
//	pe := nn.NewSinusoidalPositionalEncoding(512, 256, backend)
//	encodings := pe.Forward(100)  // [1, 100, 256]
//
//	// Add to embeddings
//	embeddings := embeddings.Add(encodings)
type SinusoidalPositionalEncoding[B tensor.Backend] = nn.SinusoidalPositionalEncoding[B]

// NewSinusoidalPositionalEncoding creates a new sinusoidal positional encoding layer.
//
// Pre-computes all positional encodings up to maxLen using sine and cosine functions.
//
// Parameters:
//   - maxLen: Maximum sequence length
//   - dim: Embedding dimension
//   - backend: Computation backend
//
// Example:
//
//	pe := nn.NewSinusoidalPositionalEncoding(512, 256, backend)
func NewSinusoidalPositionalEncoding[B tensor.Backend](maxLen, dim int, backend B) *SinusoidalPositionalEncoding[B] {
	return nn.NewSinusoidalPositionalEncoding(maxLen, dim, backend)
}

// LearnedPositionalEmbedding implements learned positional embeddings.
//
// These embeddings are trainable parameters that are updated during training.
// Used in GPT-2 and other models.
//
// Example:
//
//	backend := cpu.New()
//	pe := nn.NewLearnedPositionalEmbedding(512, 256, backend)
//	encodings := pe.Forward(100)  // [1, 100, 256]
//
//	// Get parameters for optimizer
//	params := pe.Parameters()
type LearnedPositionalEmbedding[B tensor.Backend] = nn.LearnedPositionalEmbedding[B]

// NewLearnedPositionalEmbedding creates a new learned positional embedding layer.
//
// The embeddings are initialized from a normal distribution N(0, 1).
//
// Parameters:
//   - maxLen: Maximum sequence length
//   - dim: Embedding dimension
//   - backend: Computation backend
//
// Example:
//
//	pe := nn.NewLearnedPositionalEmbedding(512, 256, backend)
func NewLearnedPositionalEmbedding[B tensor.Backend](maxLen, dim int, backend B) *LearnedPositionalEmbedding[B] {
	return nn.NewLearnedPositionalEmbedding(maxLen, dim, backend)
}

// ALiBi implements Attention with Linear Biases.
//
// ALiBi adds a linear bias to attention scores based on the distance between positions.
// Used in BLOOM, MPT, and other models. Allows extrapolation to longer sequences.
//
// Example:
//
//	backend := cpu.New()
//	alibi := nn.NewALiBi(8, backend)  // 8 attention heads
//	bias := alibi.GetBias(128)        // [1, 8, 128, 128]
//
//	// In attention:
//	scores := Q.BatchMatMul(K.T())
//	scores = scores.Add(bias)
//	weights := scores.Softmax(-1)
type ALiBi[B tensor.Backend] = nn.ALiBi[B]

// NewALiBi creates a new ALiBi bias generator.
//
// Computes slopes for each attention head using a geometric sequence.
//
// Parameters:
//   - numHeads: Number of attention heads
//   - backend: Computation backend
//
// Example:
//
//	alibi := nn.NewALiBi(8, backend)
//	bias := alibi.GetBias(64)  // Get bias for sequence length 64
func NewALiBi[B tensor.Backend](numHeads int, backend B) *ALiBi[B] {
	return nn.NewALiBi(numHeads, backend)
}
