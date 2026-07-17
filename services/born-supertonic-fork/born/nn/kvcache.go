package nn

import (
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// KVCache is a public alias for internal KV cache implementation.
//
// KVCache stores key-value pairs for efficient autoregressive generation.
// See internal/nn/kvcache.go for detailed documentation.
type KVCache[B tensor.Backend] = nn.KVCache[B]

// NewKVCache creates a new KV cache.
//
// This is a convenience wrapper for the internal implementation.
// See internal/nn.NewKVCache for detailed documentation.
func NewKVCache[B tensor.Backend](
	batchSize, numHeads, maxSeqLen, headDim int,
	backend B,
) *KVCache[B] {
	return nn.NewKVCache(batchSize, numHeads, maxSeqLen, headDim, backend)
}
