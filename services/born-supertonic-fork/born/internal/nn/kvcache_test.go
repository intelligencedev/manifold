package nn

import (
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

func TestKVCache_Update_And_Get(t *testing.T) {
	backend := cpu.New()
	batch, heads, maxSeq, headDim := 2, 4, 100, 16

	cache := NewKVCache[*cpu.CPUBackend](batch, heads, maxSeq, headDim, backend)

	// Initially empty
	if cache.Len() != 0 {
		t.Errorf("Expected empty cache, got length %d", cache.Len())
	}

	// Add first token
	k1 := tensor.Randn[float32](tensor.Shape{batch, heads, 1, headDim}, backend)
	v1 := tensor.Randn[float32](tensor.Shape{batch, heads, 1, headDim}, backend)
	cache.Update(k1, v1)

	if cache.Len() != 1 {
		t.Errorf("Expected length 1, got %d", cache.Len())
	}

	// Get and verify shape
	cachedK, cachedV := cache.Get()
	expectedShape := tensor.Shape{batch, heads, 1, headDim}
	if !shapeEqual(cachedK.Shape(), expectedShape) {
		t.Errorf("Expected keys shape %v, got %v", expectedShape, cachedK.Shape())
	}
	if !shapeEqual(cachedV.Shape(), expectedShape) {
		t.Errorf("Expected values shape %v, got %v", expectedShape, cachedV.Shape())
	}

	// Add second token
	k2 := tensor.Randn[float32](tensor.Shape{batch, heads, 1, headDim}, backend)
	v2 := tensor.Randn[float32](tensor.Shape{batch, heads, 1, headDim}, backend)
	cache.Update(k2, v2)

	if cache.Len() != 2 {
		t.Errorf("Expected length 2, got %d", cache.Len())
	}

	// Get and verify shape (concatenated)
	cachedK, cachedV = cache.Get()
	expectedShape = tensor.Shape{batch, heads, 2, headDim}
	if !shapeEqual(cachedK.Shape(), expectedShape) {
		t.Errorf("Expected keys shape %v, got %v", expectedShape, cachedK.Shape())
	}
	if !shapeEqual(cachedV.Shape(), expectedShape) {
		t.Errorf("Expected values shape %v, got %v", expectedShape, cachedV.Shape())
	}
}

func TestKVCache_Update_MultipleTokens(t *testing.T) {
	backend := cpu.New()
	batch, heads, maxSeq, headDim := 1, 2, 100, 8

	cache := NewKVCache[*cpu.CPUBackend](batch, heads, maxSeq, headDim, backend)

	// Add 5 tokens at once
	k := tensor.Randn[float32](tensor.Shape{batch, heads, 5, headDim}, backend)
	v := tensor.Randn[float32](tensor.Shape{batch, heads, 5, headDim}, backend)
	cache.Update(k, v)

	if cache.Len() != 5 {
		t.Errorf("Expected length 5, got %d", cache.Len())
	}

	// Add another 3 tokens
	k2 := tensor.Randn[float32](tensor.Shape{batch, heads, 3, headDim}, backend)
	v2 := tensor.Randn[float32](tensor.Shape{batch, heads, 3, headDim}, backend)
	cache.Update(k2, v2)

	if cache.Len() != 8 {
		t.Errorf("Expected length 8, got %d", cache.Len())
	}

	cachedK, cachedV := cache.Get()
	expectedShape := tensor.Shape{batch, heads, 8, headDim}
	if !shapeEqual(cachedK.Shape(), expectedShape) {
		t.Errorf("Expected keys shape %v, got %v", expectedShape, cachedK.Shape())
	}
	if !shapeEqual(cachedV.Shape(), expectedShape) {
		t.Errorf("Expected values shape %v, got %v", expectedShape, cachedV.Shape())
	}
}

func TestKVCache_Reset(t *testing.T) {
	backend := cpu.New()
	batch, heads, maxSeq, headDim := 2, 4, 100, 16

	cache := NewKVCache[*cpu.CPUBackend](batch, heads, maxSeq, headDim, backend)

	// Add some tokens
	k := tensor.Randn[float32](tensor.Shape{batch, heads, 10, headDim}, backend)
	v := tensor.Randn[float32](tensor.Shape{batch, heads, 10, headDim}, backend)
	cache.Update(k, v)

	if cache.Len() != 10 {
		t.Errorf("Expected length 10, got %d", cache.Len())
	}

	// Reset
	cache.Reset()

	if cache.Len() != 0 {
		t.Errorf("Expected empty cache after reset, got length %d", cache.Len())
	}

	// Can add new tokens after reset
	k2 := tensor.Randn[float32](tensor.Shape{batch, heads, 5, headDim}, backend)
	v2 := tensor.Randn[float32](tensor.Shape{batch, heads, 5, headDim}, backend)
	cache.Update(k2, v2)

	if cache.Len() != 5 {
		t.Errorf("Expected length 5 after reset and new update, got %d", cache.Len())
	}
}

func TestKVCache_Overflow(t *testing.T) {
	backend := cpu.New()
	batch, heads, maxSeq, headDim := 1, 2, 10, 8 // Small max_seq

	cache := NewKVCache[*cpu.CPUBackend](batch, heads, maxSeq, headDim, backend)

	// Add tokens up to max
	k1 := tensor.Randn[float32](tensor.Shape{batch, heads, 8, headDim}, backend)
	v1 := tensor.Randn[float32](tensor.Shape{batch, heads, 8, headDim}, backend)
	cache.Update(k1, v1)

	// Try to add more than max (8 + 5 = 13 > 10)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic on cache overflow, but didn't get one")
		}
	}()

	k2 := tensor.Randn[float32](tensor.Shape{batch, heads, 5, headDim}, backend)
	v2 := tensor.Randn[float32](tensor.Shape{batch, heads, 5, headDim}, backend)
	cache.Update(k2, v2) // Should panic
}

func TestKVCache_GetEmpty(t *testing.T) {
	backend := cpu.New()
	cache := NewKVCache[*cpu.CPUBackend](1, 2, 10, 8, backend)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when getting from empty cache, but didn't get one")
		}
	}()

	cache.Get() // Should panic
}

func TestMHA_ForwardWithCache_Single_Token(t *testing.T) {
	backend := cpu.New()
	embedDim := 64
	numHeads := 4
	headDim := embedDim / numHeads
	batch := 2
	maxSeq := 100

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)
	cache := NewKVCache[*cpu.CPUBackend](batch, numHeads, maxSeq, headDim, backend)

	// Generate first token
	token1 := tensor.Randn[float32](tensor.Shape{batch, 1, embedDim}, backend)
	out1 := mha.ForwardWithCache(token1, cache)

	// Verify output shape
	expectedShape := tensor.Shape{batch, 1, embedDim}
	if !shapeEqual(out1.Shape(), expectedShape) {
		t.Errorf("Expected output shape %v, got %v", expectedShape, out1.Shape())
	}

	// Verify cache updated
	if cache.Len() != 1 {
		t.Errorf("Expected cache length 1, got %d", cache.Len())
	}

	// Generate second token
	token2 := tensor.Randn[float32](tensor.Shape{batch, 1, embedDim}, backend)
	out2 := mha.ForwardWithCache(token2, cache)

	// Verify output shape
	if !shapeEqual(out2.Shape(), expectedShape) {
		t.Errorf("Expected output shape %v, got %v", expectedShape, out2.Shape())
	}

	// Verify cache updated
	if cache.Len() != 2 {
		t.Errorf("Expected cache length 2, got %d", cache.Len())
	}
}

func TestMHA_ForwardWithCache_Sequential_Generation(t *testing.T) {
	backend := cpu.New()
	embedDim := 32
	numHeads := 2
	headDim := embedDim / numHeads
	batch := 1
	maxSeq := 20
	numTokens := 10

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)
	cache := NewKVCache[*cpu.CPUBackend](batch, numHeads, maxSeq, headDim, backend)

	// Generate sequence token by token
	for i := 0; i < numTokens; i++ {
		token := tensor.Randn[float32](tensor.Shape{batch, 1, embedDim}, backend)
		output := mha.ForwardWithCache(token, cache)

		// Verify output shape
		expectedShape := tensor.Shape{batch, 1, embedDim}
		if !shapeEqual(output.Shape(), expectedShape) {
			t.Errorf("Token %d: Expected output shape %v, got %v", i, expectedShape, output.Shape())
		}

		// Verify cache length
		expectedLen := i + 1
		if cache.Len() != expectedLen {
			t.Errorf("Token %d: Expected cache length %d, got %d", i, expectedLen, cache.Len())
		}
	}

	// Final cache should have all tokens
	if cache.Len() != numTokens {
		t.Errorf("Expected final cache length %d, got %d", numTokens, cache.Len())
	}
}

func TestMHA_ForwardWithCache_OutputShape(t *testing.T) {
	backend := cpu.New()
	embedDim := 64
	numHeads := 4
	headDim := embedDim / numHeads
	batch := 1
	seqLen := 5

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)

	// Generate input sequence token by token
	cache := NewKVCache[*cpu.CPUBackend](batch, numHeads, seqLen, headDim, backend)
	outputsCached := make([]*tensor.Tensor[float32, *cpu.CPUBackend], 0, seqLen)

	for i := 0; i < seqLen; i++ {
		token := tensor.Randn[float32](tensor.Shape{batch, 1, embedDim}, backend)
		output := mha.ForwardWithCache(token, cache)
		outputsCached = append(outputsCached, output)
	}

	// Concatenate cached outputs
	outputCached := tensor.Cat(outputsCached, 1)

	// Verify final shape
	expectedShape := tensor.Shape{batch, seqLen, embedDim}
	if !shapeEqual(outputCached.Shape(), expectedShape) {
		t.Errorf("Expected output shape %v, got %v",
			expectedShape, outputCached.Shape())
	}

	// Note: Testing exact equivalence with standard Forward is complex
	// due to lack of slice operation. This test verifies the output shape
	// and that the cached forward completes successfully.
}

func TestMHA_ForwardWithCache_Reset(t *testing.T) {
	backend := cpu.New()
	embedDim := 32
	numHeads := 2
	headDim := embedDim / numHeads
	batch := 1
	maxSeq := 50

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)
	cache := NewKVCache[*cpu.CPUBackend](batch, numHeads, maxSeq, headDim, backend)

	// Generate first sequence
	for i := 0; i < 5; i++ {
		token := tensor.Randn[float32](tensor.Shape{batch, 1, embedDim}, backend)
		mha.ForwardWithCache(token, cache)
	}

	if cache.Len() != 5 {
		t.Errorf("Expected cache length 5, got %d", cache.Len())
	}

	// Reset cache
	cache.Reset()

	if cache.Len() != 0 {
		t.Errorf("Expected empty cache after reset, got length %d", cache.Len())
	}

	// Generate new sequence
	for i := 0; i < 3; i++ {
		token := tensor.Randn[float32](tensor.Shape{batch, 1, embedDim}, backend)
		mha.ForwardWithCache(token, cache)
	}

	if cache.Len() != 3 {
		t.Errorf("Expected cache length 3 after reset and new sequence, got %d", cache.Len())
	}
}

// Benchmark: Cached vs Non-cached generation.
func BenchmarkMHA_WithoutCache_10Tokens(b *testing.B) {
	backend := cpu.New()
	embedDim := 256
	numHeads := 8
	batch := 1
	numTokens := 10

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Without cache: recompute full sequence for each new token
		for t := 1; t <= numTokens; t++ {
			input := tensor.Randn[float32](tensor.Shape{batch, t, embedDim}, backend)
			mha.Forward(input, input, input, nil)
		}
	}
}

func BenchmarkMHA_WithCache_10Tokens(b *testing.B) {
	backend := cpu.New()
	embedDim := 256
	numHeads := 8
	headDim := embedDim / numHeads
	batch := 1
	numTokens := 10

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache := NewKVCache[*cpu.CPUBackend](batch, numHeads, 100, headDim, backend)
		// With cache: only compute new token
		for t := 0; t < numTokens; t++ {
			token := tensor.Randn[float32](tensor.Shape{batch, 1, embedDim}, backend)
			mha.ForwardWithCache(token, cache)
		}
	}
}

func BenchmarkMHA_WithoutCache_50Tokens(b *testing.B) {
	backend := cpu.New()
	embedDim := 256
	numHeads := 8
	batch := 1
	numTokens := 50

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for t := 1; t <= numTokens; t++ {
			input := tensor.Randn[float32](tensor.Shape{batch, t, embedDim}, backend)
			mha.Forward(input, input, input, nil)
		}
	}
}

func BenchmarkMHA_WithCache_50Tokens(b *testing.B) {
	backend := cpu.New()
	embedDim := 256
	numHeads := 8
	headDim := embedDim / numHeads
	batch := 1
	numTokens := 50

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache := NewKVCache[*cpu.CPUBackend](batch, numHeads, 200, headDim, backend)
		for t := 0; t < numTokens; t++ {
			token := tensor.Randn[float32](tensor.Shape{batch, 1, embedDim}, backend)
			mha.ForwardWithCache(token, cache)
		}
	}
}

func BenchmarkMHA_WithoutCache_100Tokens(b *testing.B) {
	backend := cpu.New()
	embedDim := 256
	numHeads := 8
	batch := 1
	numTokens := 100

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for t := 1; t <= numTokens; t++ {
			input := tensor.Randn[float32](tensor.Shape{batch, t, embedDim}, backend)
			mha.Forward(input, input, input, nil)
		}
	}
}

func BenchmarkMHA_WithCache_100Tokens(b *testing.B) {
	backend := cpu.New()
	embedDim := 256
	numHeads := 8
	headDim := embedDim / numHeads
	batch := 1
	numTokens := 100

	mha := NewMultiHeadAttention[*cpu.CPUBackend](embedDim, numHeads, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache := NewKVCache[*cpu.CPUBackend](batch, numHeads, 200, headDim, backend)
		for t := 0; t < numTokens; t++ {
			token := tensor.Randn[float32](tensor.Shape{batch, 1, embedDim}, backend)
			mha.ForwardWithCache(token, cache)
		}
	}
}

// Note: shapeEqual is defined in attention_test.go
