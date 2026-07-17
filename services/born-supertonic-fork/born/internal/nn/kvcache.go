package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// KVCache stores key-value pairs for efficient autoregressive generation.
//
// Without cache: O(n²) computation for generating n tokens (recompute K,V for all previous tokens)
// With cache: O(n) computation (only compute K,V for new token and append to cache)
//
// This can provide 10-100x speedup for inference depending on sequence length.
//
// Example:
//
//	cache := nn.NewKVCache[B](2, 8, 512, 64, backend) // batch=2, heads=8, maxSeq=512, headDim=64
//	for pos := 0; pos < numTokens; pos++ {
//	    output := mha.ForwardWithCache(queryToken, cache, pos)
//	}
type KVCache[B tensor.Backend] struct {
	keys    []*tensor.Tensor[float32, B] // List of key tensors
	values  []*tensor.Tensor[float32, B] // List of value tensors
	length  int                          // Current sequence length in cache
	maxLen  int                          // Maximum sequence length
	backend B
}

// NewKVCache creates a new KV cache.
//
// Parameters:
//   - batchSize: Batch size (reserved for future use)
//   - numHeads: Number of attention heads (reserved for future use)
//   - maxSeqLen: Maximum sequence length
//   - headDim: Dimension per attention head (reserved for future use)
//   - backend: Computation backend
//
// The cache starts empty (length=0) and grows as key-value pairs are added.
// Note: batchSize, numHeads, and headDim are not currently used but kept for API consistency.
//
// Example:
//
//	cache := nn.NewKVCache[B](2, 8, 512, 64, backend)
func NewKVCache[B tensor.Backend](
	_ /* batchSize */, _ /* numHeads */, maxSeqLen int, _ /* headDim */ int,
	backend B,
) *KVCache[B] {
	return &KVCache[B]{
		keys:    make([]*tensor.Tensor[float32, B], 0, maxSeqLen),
		values:  make([]*tensor.Tensor[float32, B], 0, maxSeqLen),
		length:  0,
		maxLen:  maxSeqLen,
		backend: backend,
	}
}

// Update adds new key-value pairs to the cache at the current position.
//
// Parameters:
//   - key: New key tensor [batch, num_heads, seq_len, head_dim]
//   - value: New value tensor [batch, num_heads, seq_len, head_dim]
//
// The new tensors are appended to the cache and the length is updated.
// Panics if the cache would exceed maxLen.
//
// Example:
//
//	// Add single token (seq_len=1)
//	cache.Update(key, value) // key/value: [2, 8, 1, 64]
//	// Add multiple tokens (seq_len=10)
//	cache.Update(key, value) // key/value: [2, 8, 10, 64]
func (c *KVCache[B]) Update(key, value *tensor.Tensor[float32, B]) {
	seqLen := key.Shape()[2] // [batch, num_heads, seq_len, head_dim]
	if c.length+seqLen > c.maxLen {
		panic(fmt.Sprintf("KVCache: cache overflow (length=%d + new=%d > max=%d)",
			c.length, seqLen, c.maxLen))
	}

	c.keys = append(c.keys, key)
	c.values = append(c.values, value)
	c.length += seqLen
}

// Get returns cached keys and values up to the current length.
//
// Returns:
//   - keys: [batch, num_heads, length, head_dim]
//   - values: [batch, num_heads, length, head_dim]
//
// If the cache is empty, panics.
//
// Example:
//
//	keys, values := cache.Get()
//	// keys/values: [2, 8, 15, 64] if 15 tokens were added
func (c *KVCache[B]) Get() (keys, values *tensor.Tensor[float32, B]) {
	if c.length == 0 {
		panic("KVCache: cannot get from empty cache")
	}

	// If only one tensor, return it directly (optimization)
	if len(c.keys) == 1 {
		return c.keys[0], c.values[0]
	}

	// Concatenate all cached tensors along seq dimension (dim=2)
	keys = tensor.Cat(c.keys, 2)
	values = tensor.Cat(c.values, 2)

	return keys, values
}

// Reset clears the cache for new generation.
//
// After reset, the cache is empty (length=0) and ready for new sequences.
//
// Example:
//
//	cache.Reset() // Clear cache
//	// Start new generation sequence
func (c *KVCache[B]) Reset() {
	c.keys = c.keys[:0]
	c.values = c.values[:0]
	c.length = 0
}

// Clear clears the cache for new generation.
//
// This is an alias for Reset provided to satisfy the generate.KVCache interface.
// After Clear, the cache is empty (length=0) and ready for a new sequence.
func (c *KVCache[B]) Clear() {
	c.Reset()
}

// Len returns the current sequence length in cache.
//
// Example:
//
//	if cache.Len() > 100 {
//	    // Generate summary or truncate
//	}
func (c *KVCache[B]) Len() int {
	return c.length
}
