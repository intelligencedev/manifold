package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestScaledDotProductAttention_Basic tests basic attention computation.
func TestScaledDotProductAttention_Basic(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Simple case: batch=1, heads=1, seq=2, head_dim=2
	// Q = [[1, 0], [0, 1]]
	// K = [[1, 0], [0, 1]]
	// V = [[2, 0], [0, 2]]
	Q, err := tensor.FromSlice[float32](
		[]float32{1, 0, 0, 1},
		tensor.Shape{1, 1, 2, 2},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create query: %v", err)
	}

	K, err := tensor.FromSlice[float32](
		[]float32{1, 0, 0, 1},
		tensor.Shape{1, 1, 2, 2},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	V, err := tensor.FromSlice[float32](
		[]float32{2, 0, 0, 2},
		tensor.Shape{1, 1, 2, 2},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create value: %v", err)
	}

	// Compute attention with auto-scale
	output, weights := ScaledDotProductAttention(Q, K, V, nil, 0)

	// Check output shape
	expectedShape := tensor.Shape{1, 1, 2, 2}
	if !shapeEqual(output.Shape(), expectedShape) {
		t.Errorf("Output shape = %v, expected %v", output.Shape(), expectedShape)
	}

	// Check weights shape
	expectedWeightsShape := tensor.Shape{1, 1, 2, 2}
	if !shapeEqual(weights.Shape(), expectedWeightsShape) {
		t.Errorf("Weights shape = %v, expected %v", weights.Shape(), expectedWeightsShape)
	}

	// Weights should sum to 1 along last dimension
	weightsData := weights.Data()
	row1Sum := weightsData[0] + weightsData[1]
	row2Sum := weightsData[2] + weightsData[3]

	if math.Abs(float64(row1Sum-1.0)) > 0.001 {
		t.Errorf("Row 1 weights sum = %v, expected 1.0", row1Sum)
	}
	if math.Abs(float64(row2Sum-1.0)) > 0.001 {
		t.Errorf("Row 2 weights sum = %v, expected 1.0", row2Sum)
	}
}

// TestScaledDotProductAttention_WithCausalMask tests causal attention.
func TestScaledDotProductAttention_WithCausalMask(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create random Q, K, V
	seqLen := 4
	headDim := 8
	Q := tensor.Randn[float32](tensor.Shape{1, 1, seqLen, headDim}, backend)
	K := tensor.Randn[float32](tensor.Shape{1, 1, seqLen, headDim}, backend)
	V := tensor.Randn[float32](tensor.Shape{1, 1, seqLen, headDim}, backend)

	// Create causal mask
	mask := CausalMask(seqLen, backend)

	// Compute attention
	_, weights := ScaledDotProductAttention(Q, K, V, mask, 0)

	// Check that weights are 0 for future positions
	weightsData := weights.Data()

	// weights shape: [1, 1, seq_len, seq_len]
	// For each position i, weights to positions j > i should be 0
	for i := 0; i < seqLen; i++ {
		for j := 0; j < seqLen; j++ {
			idx := i*seqLen + j
			weight := weightsData[idx]

			if j > i {
				// Future position - should be 0 (or very close)
				if math.Abs(float64(weight)) > 1e-6 {
					t.Errorf("Position %d attending to future %d: weight = %v, expected ~0", i, j, weight)
				}
			} else {
				// Past/current position - should be > 0
				if weight < 0 {
					t.Errorf("Position %d attending to %d: negative weight %v", i, j, weight)
				}
			}
		}
	}

	// Each row should sum to 1
	for i := 0; i < seqLen; i++ {
		sum := float32(0)
		for j := 0; j < seqLen; j++ {
			idx := i*seqLen + j
			sum += weightsData[idx]
		}
		if math.Abs(float64(sum-1.0)) > 0.001 {
			t.Errorf("Row %d weights sum = %v, expected 1.0", i, sum)
		}
	}
}

// TestScaledDotProductAttention_CrossAttention tests cross-attention (seq_q != seq_k).
func TestScaledDotProductAttention_CrossAttention(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Cross-attention: query from decoder, key/value from encoder
	seqQ := 5  // decoder sequence length
	seqKV := 7 // encoder sequence length
	headDim := 16

	Q := tensor.Randn[float32](tensor.Shape{2, 4, seqQ, headDim}, backend) // batch=2, heads=4
	K := tensor.Randn[float32](tensor.Shape{2, 4, seqKV, headDim}, backend)
	V := tensor.Randn[float32](tensor.Shape{2, 4, seqKV, headDim}, backend)

	// Compute attention
	output, weights := ScaledDotProductAttention(Q, K, V, nil, 0)

	// Check shapes
	expectedOutputShape := tensor.Shape{2, 4, seqQ, headDim}
	if !shapeEqual(output.Shape(), expectedOutputShape) {
		t.Errorf("Output shape = %v, expected %v", output.Shape(), expectedOutputShape)
	}

	expectedWeightsShape := tensor.Shape{2, 4, seqQ, seqKV}
	if !shapeEqual(weights.Shape(), expectedWeightsShape) {
		t.Errorf("Weights shape = %v, expected %v", weights.Shape(), expectedWeightsShape)
	}

	// Verify each query position has weights summing to 1
	weightsData := weights.Data()
	batch := 2
	heads := 4

	for b := 0; b < batch; b++ {
		for h := 0; h < heads; h++ {
			for q := 0; q < seqQ; q++ {
				sum := float32(0)
				for k := 0; k < seqKV; k++ {
					// Index: [b, h, q, k]
					idx := b*heads*seqQ*seqKV + h*seqQ*seqKV + q*seqKV + k
					sum += weightsData[idx]
				}
				if math.Abs(float64(sum-1.0)) > 0.01 {
					t.Errorf("Batch %d, head %d, query %d: weights sum = %v, expected 1.0",
						b, h, q, sum)
					break // Only report first error per batch/head
				}
			}
		}
	}
}

// TestScaledDotProductAttention_CustomScale tests custom scaling factor.
func TestScaledDotProductAttention_CustomScale(t *testing.T) {
	backend := autodiff.New(cpu.New())

	Q := tensor.Randn[float32](tensor.Shape{1, 1, 3, 8}, backend)
	K := tensor.Randn[float32](tensor.Shape{1, 1, 3, 8}, backend)
	V := tensor.Randn[float32](tensor.Shape{1, 1, 3, 8}, backend)

	// Test with custom scale (should not panic)
	customScale := float32(0.5)
	output, weights := ScaledDotProductAttention(Q, K, V, nil, customScale)

	// Basic sanity checks
	if output == nil || weights == nil {
		t.Error("ScaledDotProductAttention returned nil")
	}

	// Weights should sum to 1
	weightsData := weights.Data()
	for i := 0; i < 3; i++ {
		sum := float32(0)
		for j := 0; j < 3; j++ {
			idx := i*3 + j
			sum += weightsData[idx]
		}
		if math.Abs(float64(sum-1.0)) > 0.001 {
			t.Errorf("Row %d weights sum = %v, expected 1.0", i, sum)
		}
	}
}

// TestCausalMask_Shape tests causal mask shape.
func TestCausalMask_Shape(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		seqLen int
		want   tensor.Shape
	}{
		{1, tensor.Shape{1, 1, 1, 1}},
		{4, tensor.Shape{1, 1, 4, 4}},
		{10, tensor.Shape{1, 1, 10, 10}},
	}

	for _, tt := range tests {
		mask := CausalMask(tt.seqLen, backend)
		if !shapeEqual(mask.Shape(), tt.want) {
			t.Errorf("CausalMask(%d) shape = %v, want %v", tt.seqLen, mask.Shape(), tt.want)
		}
	}
}

// TestCausalMask_Values tests causal mask values.
func TestCausalMask_Values(t *testing.T) {
	backend := cpu.New()

	seqLen := 4
	mask := CausalMask(seqLen, backend)
	data := mask.Data()

	// Expected:
	// [[0,   -inf, -inf, -inf],
	//  [0,   0,    -inf, -inf],
	//  [0,   0,    0,    -inf],
	//  [0,   0,    0,    0   ]]

	for i := 0; i < seqLen; i++ {
		for j := 0; j < seqLen; j++ {
			idx := i*seqLen + j
			val := data[idx]

			if j > i {
				// Upper triangle - should be -inf
				if !math.IsInf(float64(val), -1) {
					t.Errorf("Mask[%d,%d] = %v, expected -inf", i, j, val)
				}
			} else {
				// Lower triangle + diagonal - should be 0
				if val != 0 {
					t.Errorf("Mask[%d,%d] = %v, expected 0", i, j, val)
				}
			}
		}
	}
}

// TestScaledDotProductAttention_InvalidInputs tests error handling.
func TestScaledDotProductAttention_InvalidInputs(t *testing.T) {
	backend := cpu.New()

	// Test 3D query (should panic)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for 3D query, got none")
		}
	}()

	Q := tensor.Randn[float32](tensor.Shape{2, 3, 4}, backend)
	K := tensor.Randn[float32](tensor.Shape{1, 1, 3, 4}, backend)
	V := tensor.Randn[float32](tensor.Shape{1, 1, 3, 4}, backend)

	ScaledDotProductAttention(Q, K, V, nil, 0)
}

// TestScaledDotProductAttention_HeadDimMismatch tests head_dim validation.
func TestScaledDotProductAttention_HeadDimMismatch(t *testing.T) {
	backend := cpu.New()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for head_dim mismatch, got none")
		}
	}()

	Q := tensor.Randn[float32](tensor.Shape{1, 1, 3, 8}, backend)  // head_dim=8
	K := tensor.Randn[float32](tensor.Shape{1, 1, 3, 16}, backend) // head_dim=16
	V := tensor.Randn[float32](tensor.Shape{1, 1, 3, 16}, backend)

	ScaledDotProductAttention(Q, K, V, nil, 0)
}

// TestScaledDotProductAttention_SeqLenMismatch tests seq_len validation for K and V.
func TestScaledDotProductAttention_SeqLenMismatch(t *testing.T) {
	backend := cpu.New()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for K/V seq_len mismatch, got none")
		}
	}()

	Q := tensor.Randn[float32](tensor.Shape{1, 1, 5, 8}, backend)
	K := tensor.Randn[float32](tensor.Shape{1, 1, 3, 8}, backend) // seq=3
	V := tensor.Randn[float32](tensor.Shape{1, 1, 7, 8}, backend) // seq=7 (mismatch!)

	ScaledDotProductAttention(Q, K, V, nil, 0)
}

// BenchmarkScaledDotProductAttention benchmarks attention computation.
func BenchmarkScaledDotProductAttention(b *testing.B) {
	backend := cpu.New()

	// Realistic transformer sizes
	Q := tensor.Randn[float32](tensor.Shape{8, 12, 512, 64}, backend) // batch=8, heads=12, seq=512, dim=64
	K := tensor.Randn[float32](tensor.Shape{8, 12, 512, 64}, backend)
	V := tensor.Randn[float32](tensor.Shape{8, 12, 512, 64}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScaledDotProductAttention(Q, K, V, nil, 0)
	}
}

// BenchmarkScaledDotProductAttention_WithMask benchmarks attention with causal mask.
func BenchmarkScaledDotProductAttention_WithMask(b *testing.B) {
	backend := cpu.New()

	seqLen := 512
	Q := tensor.Randn[float32](tensor.Shape{8, 12, seqLen, 64}, backend)
	K := tensor.Randn[float32](tensor.Shape{8, 12, seqLen, 64}, backend)
	V := tensor.Randn[float32](tensor.Shape{8, 12, seqLen, 64}, backend)
	mask := CausalMask(seqLen, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScaledDotProductAttention(Q, K, V, mask, 0)
	}
}

// BenchmarkCausalMask benchmarks causal mask creation.
func BenchmarkCausalMask(b *testing.B) {
	backend := cpu.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CausalMask(512, backend)
	}
}

// Helper function to check if two shapes are equal.
func shapeEqual(a, b tensor.Shape) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
