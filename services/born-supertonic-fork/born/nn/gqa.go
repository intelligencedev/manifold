// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package nn

import (
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// GQAConfig configures a GroupedQueryAttention layer.
type GQAConfig = nn.GQAConfig

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
type GroupedQueryAttention[B tensor.Backend] = nn.GroupedQueryAttention[B]

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
//	output := gqa.Forward(x, cache, startPos)
func NewGQA[B tensor.Backend](cfg GQAConfig, backend B) *GroupedQueryAttention[B] {
	return nn.NewGQA(cfg, backend)
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
func RepeatKV[B tensor.Backend](kv *tensor.Tensor[float32, B], nRep int) *tensor.Tensor[float32, B] {
	return nn.RepeatKV(kv, nRep)
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
	return nn.MQA(embedDim, nQHeads, headDim)
}
