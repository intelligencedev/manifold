package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestMultiHeadAttention_SelfAttention tests self-attention (Q=K=V).
func TestMultiHeadAttention_SelfAttention(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create MHA with 768 dim, 12 heads -> head_dim = 64
	embedDim := 768
	numHeads := 12
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	// Input: [batch=2, seq=10, embed_dim=768]
	batch := 2
	seq := 10
	input := tensor.Randn[float32](tensor.Shape{batch, seq, embedDim}, backend)

	// Self-attention: Q=K=V
	output := mha.Forward(input, input, input, nil)

	// Verify output shape
	expectedShape := tensor.Shape{batch, seq, embedDim}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Expected output shape %v, got %v", expectedShape, output.Shape())
	}
}

// TestMultiHeadAttention_CrossAttention tests cross-attention (Q != K/V).
func TestMultiHeadAttention_CrossAttention(t *testing.T) {
	backend := autodiff.New(cpu.New())

	embedDim := 256
	numHeads := 8
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	batch := 2
	seqQ := 10
	seqKV := 20

	// Query: [2, 10, 256]
	query := tensor.Randn[float32](tensor.Shape{batch, seqQ, embedDim}, backend)

	// Key/Value: [2, 20, 256]
	key := tensor.Randn[float32](tensor.Shape{batch, seqKV, embedDim}, backend)
	value := tensor.Randn[float32](tensor.Shape{batch, seqKV, embedDim}, backend)

	// Cross-attention
	output := mha.Forward(query, key, value, nil)

	// Output should match query sequence length
	expectedShape := tensor.Shape{batch, seqQ, embedDim}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Expected output shape %v, got %v", expectedShape, output.Shape())
	}
}

// TestMultiHeadAttention_WithMask tests attention with causal mask.
func TestMultiHeadAttention_WithMask(t *testing.T) {
	backend := autodiff.New(cpu.New())

	embedDim := 128
	numHeads := 4
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	batch := 1
	seq := 5

	input := tensor.Randn[float32](tensor.Shape{batch, seq, embedDim}, backend)

	// Create causal mask [1, 1, seq, seq]
	mask := CausalMask(seq, backend)

	// Forward with mask
	output := mha.Forward(input, input, input, mask)

	// Verify output shape
	expectedShape := tensor.Shape{batch, seq, embedDim}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Expected output shape %v, got %v", expectedShape, output.Shape())
	}

	// Output should be finite (no NaNs/Infs from mask)
	outputData := output.Data()
	for i, v := range outputData {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Errorf("Output contains NaN/Inf at index %d: %v", i, v)
		}
	}
}

// TestMultiHeadAttention_ParameterCount verifies parameter count.
func TestMultiHeadAttention_ParameterCount(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Standard BERT-base config: 768 dim, 12 heads
	embedDim := 768
	numHeads := 12
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	params := mha.Parameters()

	// Should have 8 parameters: WQ.weight, WQ.bias, WK.weight, WK.bias, WV.weight, WV.bias, WO.weight, WO.bias
	expectedParamCount := 8
	if len(params) != expectedParamCount {
		t.Errorf("Expected %d parameters, got %d", expectedParamCount, len(params))
	}

	// Count total parameter elements
	totalParams := 0
	for _, p := range params {
		totalParams += p.Tensor().Shape().NumElements()
	}

	// Each Linear has embedDim * embedDim weights + embedDim biases
	// WQ, WK, WV, WO: 4 * (768*768 + 768) = 4 * 590,592 = 2,362,368
	expectedTotal := 4 * (embedDim*embedDim + embedDim)
	if totalParams != expectedTotal {
		t.Errorf("Expected %d total parameters, got %d", expectedTotal, totalParams)
	}

	t.Logf("Total parameters: %d (expected: %d)", totalParams, expectedTotal)
}

// TestMultiHeadAttention_SingleHead tests MHA with single head (equivalent to standard attention).
func TestMultiHeadAttention_SingleHead(t *testing.T) {
	backend := autodiff.New(cpu.New())

	embedDim := 64
	numHeads := 1 // Single head
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	batch := 1
	seq := 3

	input := tensor.Randn[float32](tensor.Shape{batch, seq, embedDim}, backend)

	output := mha.Forward(input, input, input, nil)

	expectedShape := tensor.Shape{batch, seq, embedDim}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Expected output shape %v, got %v", expectedShape, output.Shape())
	}
}

// TestMultiHeadAttention_DifferentSeqLengths tests with different seq lengths for Q and K/V.
func TestMultiHeadAttention_DifferentSeqLengths(t *testing.T) {
	backend := autodiff.New(cpu.New())

	embedDim := 512
	numHeads := 8
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	batch := 3
	seqQ := 7
	seqKV := 15

	query := tensor.Randn[float32](tensor.Shape{batch, seqQ, embedDim}, backend)
	key := tensor.Randn[float32](tensor.Shape{batch, seqKV, embedDim}, backend)
	value := tensor.Randn[float32](tensor.Shape{batch, seqKV, embedDim}, backend)

	output := mha.Forward(query, key, value, nil)

	// Output seq length should match query
	expectedShape := tensor.Shape{batch, seqQ, embedDim}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Expected output shape %v, got %v", expectedShape, output.Shape())
	}
}

// TestMultiHeadAttention_EmbedDimNotDivisible tests panic when embed_dim not divisible by num_heads.
func TestMultiHeadAttention_EmbedDimNotDivisible(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// embedDim=100, numHeads=7 -> 100 % 7 != 0
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when embed_dim not divisible by num_heads")
		}
	}()

	NewMultiHeadAttention(100, 7, backend)
}

// TestMultiHeadAttention_ForwardWithWeights tests ForwardWithWeights.
func TestMultiHeadAttention_ForwardWithWeights(t *testing.T) {
	backend := autodiff.New(cpu.New())

	embedDim := 256
	numHeads := 8
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	batch := 2
	seq := 10

	input := tensor.Randn[float32](tensor.Shape{batch, seq, embedDim}, backend)

	output, weights := mha.ForwardWithWeights(input, input, input, nil)

	// Verify output shape
	expectedOutputShape := tensor.Shape{batch, seq, embedDim}
	if !output.Shape().Equal(expectedOutputShape) {
		t.Errorf("Expected output shape %v, got %v", expectedOutputShape, output.Shape())
	}

	// Verify weights shape [batch, num_heads, seq_q, seq_k]
	expectedWeightsShape := tensor.Shape{batch, numHeads, seq, seq}
	if !weights.Shape().Equal(expectedWeightsShape) {
		t.Errorf("Expected weights shape %v, got %v", expectedWeightsShape, weights.Shape())
	}

	// Attention weights should sum to 1.0 along last dimension
	weightsData := weights.Data()
	for b := 0; b < batch; b++ {
		for h := 0; h < numHeads; h++ {
			for q := 0; q < seq; q++ {
				sum := float32(0.0)
				for k := 0; k < seq; k++ {
					// Index: [b, h, q, k]
					idx := b*numHeads*seq*seq + h*seq*seq + q*seq + k
					sum += weightsData[idx]
				}
				if math.Abs(float64(sum-1.0)) > 1e-5 {
					t.Errorf("Attention weights for [batch=%d, head=%d, query=%d] sum to %.6f, expected 1.0",
						b, h, q, sum)
				}
			}
		}
	}
}

// BenchmarkMultiHeadAttention_768dim_12heads benchmarks BERT-base config.
func BenchmarkMultiHeadAttention_768dim_12heads(b *testing.B) {
	backend := autodiff.New(cpu.New())

	embedDim := 768
	numHeads := 12
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	batch := 8
	seq := 128

	input := tensor.Randn[float32](tensor.Shape{batch, seq, embedDim}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mha.Forward(input, input, input, nil)
	}
}

// BenchmarkMultiHeadAttention_1024dim_16heads benchmarks larger config.
func BenchmarkMultiHeadAttention_1024dim_16heads(b *testing.B) {
	backend := autodiff.New(cpu.New())

	embedDim := 1024
	numHeads := 16
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	batch := 4
	seq := 256

	input := tensor.Randn[float32](tensor.Shape{batch, seq, embedDim}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mha.Forward(input, input, input, nil)
	}
}

// BenchmarkMultiHeadAttention_WithMask benchmarks with causal mask.
func BenchmarkMultiHeadAttention_WithMask(b *testing.B) {
	backend := autodiff.New(cpu.New())

	embedDim := 768
	numHeads := 12
	mha := NewMultiHeadAttention(embedDim, numHeads, backend)

	batch := 8
	seq := 128

	input := tensor.Randn[float32](tensor.Shape{batch, seq, embedDim}, backend)
	mask := CausalMask(seq, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mha.Forward(input, input, input, mask)
	}
}
