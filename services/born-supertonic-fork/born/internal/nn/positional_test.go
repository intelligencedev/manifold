package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// equalShape compares two shapes for equality.
func equalShape(a, b tensor.Shape) bool {
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

// ============================================================================
// SinusoidalPositionalEncoding Tests
// ============================================================================

func TestNewSinusoidalPositionalEncoding(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name      string
		maxLen    int
		dim       int
		wantPanic bool
	}{
		{
			name:      "valid config",
			maxLen:    512,
			dim:       256,
			wantPanic: false,
		},
		{
			name:      "invalid maxLen",
			maxLen:    0,
			dim:       256,
			wantPanic: true,
		},
		{
			name:      "invalid dim",
			maxLen:    512,
			dim:       -1,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("NewSinusoidalPositionalEncoding() panic = %v, wantPanic %v", r != nil, tt.wantPanic)
				}
			}()

			pe := NewSinusoidalPositionalEncoding(tt.maxLen, tt.dim, backend)
			if !tt.wantPanic {
				if pe.MaxLen != tt.maxLen {
					t.Errorf("MaxLen = %d, want %d", pe.MaxLen, tt.maxLen)
				}
				if pe.Dim != tt.dim {
					t.Errorf("Dim = %d, want %d", pe.Dim, tt.dim)
				}

				// Check encoding shape
				expectedShape := tensor.Shape{tt.maxLen, tt.dim}
				if !equalShape(pe.Encoding.Shape(), expectedShape) {
					t.Errorf("Encoding shape = %v, want %v", pe.Encoding.Shape(), expectedShape)
				}
			}
		})
	}
}

func TestSinusoidalPositionalEncodingForward(t *testing.T) {
	backend := cpu.New()
	pe := NewSinusoidalPositionalEncoding(100, 4, backend)

	// Test forward
	seqLen := 10
	out := pe.Forward(seqLen)

	// Check shape: [1, seqLen, dim]
	expectedShape := tensor.Shape{1, seqLen, 4}
	if !equalShape(out.Shape(), expectedShape) {
		t.Errorf("Forward() shape = %v, want %v", out.Shape(), expectedShape)
	}

	// Check values at position 0
	// PE(0, 2i) = sin(0) = 0
	// PE(0, 2i+1) = cos(0) = 1
	outData := out.Data()
	epsilon := 1e-5

	// pos=0, dim=0 (even): sin(0) = 0
	if math.Abs(float64(outData[0])) > epsilon {
		t.Errorf("PE(0, 0) = %f, want 0.0", outData[0])
	}

	// pos=0, dim=1 (odd): cos(0) = 1
	if math.Abs(float64(outData[1]-1.0)) > epsilon {
		t.Errorf("PE(0, 1) = %f, want 1.0", outData[1])
	}
}

func TestSinusoidalPositionalEncodingSeqLenTooLong(t *testing.T) {
	backend := cpu.New()
	pe := NewSinusoidalPositionalEncoding(100, 4, backend)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Forward() with seqLen > MaxLen should panic")
		}
	}()

	_ = pe.Forward(200) // 200 > 100
}

func TestSinusoidalPositionalEncodingValues(t *testing.T) {
	backend := cpu.New()
	pe := NewSinusoidalPositionalEncoding(10, 4, backend)

	out := pe.Forward(2) // Get encodings for positions 0 and 1
	outData := out.Data()

	// Position 0, all dimensions
	// PE(0, 0) = sin(0 / 10000^(0/4)) = sin(0) = 0
	// PE(0, 1) = cos(0 / 10000^(0/4)) = cos(0) = 1
	// PE(0, 2) = sin(0 / 10000^(2/4)) = sin(0) = 0
	// PE(0, 3) = cos(0 / 10000^(2/4)) = cos(0) = 1

	epsilon := 1e-5
	expected := []float32{0, 1, 0, 1} // position 0

	for i := 0; i < 4; i++ {
		if math.Abs(float64(outData[i]-expected[i])) > epsilon {
			t.Errorf("PE(0, %d) = %f, want %f", i, outData[i], expected[i])
		}
	}
}

// ============================================================================
// LearnedPositionalEmbedding Tests
// ============================================================================

func TestNewLearnedPositionalEmbedding(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name      string
		maxLen    int
		dim       int
		wantPanic bool
	}{
		{
			name:      "valid config",
			maxLen:    512,
			dim:       256,
			wantPanic: false,
		},
		{
			name:      "invalid maxLen",
			maxLen:    0,
			dim:       256,
			wantPanic: true,
		},
		{
			name:      "invalid dim",
			maxLen:    512,
			dim:       -1,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("NewLearnedPositionalEmbedding() panic = %v, wantPanic %v", r != nil, tt.wantPanic)
				}
			}()

			pe := NewLearnedPositionalEmbedding(tt.maxLen, tt.dim, backend)
			if !tt.wantPanic {
				if pe.MaxLen != tt.maxLen {
					t.Errorf("MaxLen = %d, want %d", pe.MaxLen, tt.maxLen)
				}
				if pe.Dim != tt.dim {
					t.Errorf("Dim = %d, want %d", pe.Dim, tt.dim)
				}

				// Check that embedding was created
				if pe.Embedding == nil {
					t.Error("Embedding is nil")
				}
			}
		})
	}
}

func TestLearnedPositionalEmbeddingForward(t *testing.T) {
	backend := cpu.New()
	pe := NewLearnedPositionalEmbedding(100, 16, backend)

	// Test forward
	seqLen := 10
	out := pe.Forward(seqLen)

	// Check shape: [1, seqLen, dim]
	expectedShape := tensor.Shape{1, seqLen, 16}
	if !equalShape(out.Shape(), expectedShape) {
		t.Errorf("Forward() shape = %v, want %v", out.Shape(), expectedShape)
	}
}

func TestLearnedPositionalEmbeddingSeqLenTooLong(t *testing.T) {
	backend := cpu.New()
	pe := NewLearnedPositionalEmbedding(100, 16, backend)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Forward() with seqLen > MaxLen should panic")
		}
	}()

	_ = pe.Forward(200) // 200 > 100
}

func TestLearnedPositionalEmbeddingParameters(t *testing.T) {
	backend := cpu.New()
	pe := NewLearnedPositionalEmbedding(100, 16, backend)

	params := pe.Parameters()

	// Should have 1 parameter (the embedding weight)
	if len(params) != 1 {
		t.Errorf("Parameters() count = %d, want 1", len(params))
	}

	// Check parameter shape: [maxLen, dim]
	paramShape := params[0].Tensor().Shape()
	expectedShape := tensor.Shape{100, 16}
	if !equalShape(paramShape, expectedShape) {
		t.Errorf("Parameter shape = %v, want %v", paramShape, expectedShape)
	}
}

func TestLearnedPositionalEmbeddingDifferentPositions(t *testing.T) {
	backend := cpu.New()
	pe := NewLearnedPositionalEmbedding(10, 4, backend)

	// Get embeddings for positions 0-2
	out := pe.Forward(3)
	outData := out.Data() // [1, 3, 4] -> 12 elements

	// Embeddings for different positions should be different (with high probability)
	// Check that position 0 and position 1 have different embeddings
	pos0 := outData[0:4]  // First 4 elements
	pos1 := outData[4:8]  // Next 4 elements
	pos2 := outData[8:12] // Last 4 elements

	// At least one dimension should differ (very high probability with random init)
	same01 := true
	for i := 0; i < 4; i++ {
		if math.Abs(float64(pos0[i]-pos1[i])) > 1e-6 {
			same01 = false
			break
		}
	}

	same12 := true
	for i := 0; i < 4; i++ {
		if math.Abs(float64(pos1[i]-pos2[i])) > 1e-6 {
			same12 = false
			break
		}
	}

	if same01 || same12 {
		t.Error("Different positions should have different embeddings (with very high probability)")
	}
}

// ============================================================================
// ALiBi Tests
// ============================================================================

func TestNewALiBi(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name      string
		numHeads  int
		wantPanic bool
	}{
		{
			name:      "valid heads",
			numHeads:  8,
			wantPanic: false,
		},
		{
			name:      "single head",
			numHeads:  1,
			wantPanic: false,
		},
		{
			name:      "invalid heads",
			numHeads:  0,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("NewALiBi() panic = %v, wantPanic %v", r != nil, tt.wantPanic)
				}
			}()

			alibi := NewALiBi(tt.numHeads, backend)
			if !tt.wantPanic {
				if alibi.NumHeads != tt.numHeads {
					t.Errorf("NumHeads = %d, want %d", alibi.NumHeads, tt.numHeads)
				}

				// Check slopes length
				if len(alibi.Slopes) != tt.numHeads {
					t.Errorf("Slopes length = %d, want %d", len(alibi.Slopes), tt.numHeads)
				}
			}
		})
	}
}

func TestALiBiSlopes(t *testing.T) {
	backend := cpu.New()
	alibi := NewALiBi(8, backend)

	// Check slopes are in descending order
	for i := 1; i < len(alibi.Slopes); i++ {
		if alibi.Slopes[i] >= alibi.Slopes[i-1] {
			t.Errorf("Slopes should be descending: slopes[%d]=%f >= slopes[%d]=%f",
				i, alibi.Slopes[i], i-1, alibi.Slopes[i-1])
		}
	}

	// Check first slope (should be largest)
	// For 8 heads: slope_0 = 2^(-8/8 * 1) = 2^(-1) = 0.5
	expected := float32(0.5)
	epsilon := float32(1e-5)
	if math.Abs(float64(alibi.Slopes[0]-expected)) > float64(epsilon) {
		t.Errorf("First slope = %f, want ~%f", alibi.Slopes[0], expected)
	}

	// Check last slope
	// For 8 heads: slope_7 = 2^(-8/8 * 8) = 2^(-8) ≈ 0.00390625
	expectedLast := float32(math.Pow(2, -8))
	if math.Abs(float64(alibi.Slopes[7]-expectedLast)) > float64(epsilon) {
		t.Errorf("Last slope = %f, want ~%f", alibi.Slopes[7], expectedLast)
	}
}

func TestALiBiGetBias(t *testing.T) {
	backend := cpu.New()
	alibi := NewALiBi(4, backend)

	seqLen := 8
	bias := alibi.GetBias(seqLen)

	// Check shape: [1, num_heads, seq_len, seq_len]
	expectedShape := tensor.Shape{1, 4, seqLen, seqLen}
	if !equalShape(bias.Shape(), expectedShape) {
		t.Errorf("GetBias() shape = %v, want %v", bias.Shape(), expectedShape)
	}

	biasData := bias.Data()

	// Check diagonal (i == j, distance = 0)
	// bias[h, i, i] = -slope[h] * 0 = 0
	for h := 0; h < 4; h++ {
		for i := 0; i < seqLen; i++ {
			idx := h*seqLen*seqLen + i*seqLen + i
			if math.Abs(float64(biasData[idx])) > 1e-6 {
				t.Errorf("Diagonal bias[%d, %d, %d] = %f, want 0.0", h, i, i, biasData[idx])
			}
		}
	}

	// Check symmetry (distance-based)
	// bias[h, i, j] should equal bias[h, j, i] (both are -slope * |i-j|)
	for h := 0; h < 4; h++ {
		for i := 0; i < seqLen; i++ {
			for j := 0; j < seqLen; j++ {
				idxIJ := h*seqLen*seqLen + i*seqLen + j
				idxJI := h*seqLen*seqLen + j*seqLen + i
				if math.Abs(float64(biasData[idxIJ]-biasData[idxJI])) > 1e-6 {
					t.Errorf("Bias not symmetric: bias[%d,%d,%d]=%f != bias[%d,%d,%d]=%f",
						h, i, j, biasData[idxIJ], h, j, i, biasData[idxJI])
				}
			}
		}
	}
}

func TestALiBiBiasValues(t *testing.T) {
	backend := cpu.New()
	alibi := NewALiBi(2, backend)

	seqLen := 3
	bias := alibi.GetBias(seqLen)
	biasData := bias.Data()

	// For 2 heads:
	// slope_0 = 2^(-8/2 * 1) = 2^(-4) = 0.0625
	// slope_1 = 2^(-8/2 * 2) = 2^(-8) ≈ 0.00390625

	slope0 := alibi.Slopes[0]
	slope1 := alibi.Slopes[1]

	// Check specific values
	// Head 0, position (0, 1): distance = 1, bias = -slope_0 * 1
	idx := 0*seqLen*seqLen + 0*seqLen + 1
	expected := -slope0 * 1
	if math.Abs(float64(biasData[idx]-expected)) > 1e-5 {
		t.Errorf("bias[0, 0, 1] = %f, want %f", biasData[idx], expected)
	}

	// Head 1, position (1, 2): distance = 1, bias = -slope_1 * 1
	idx = 1*seqLen*seqLen + 1*seqLen + 2
	expected = -slope1 * 1
	if math.Abs(float64(biasData[idx]-expected)) > 1e-5 {
		t.Errorf("bias[1, 1, 2] = %f, want %f", biasData[idx], expected)
	}

	// Head 0, position (0, 2): distance = 2, bias = -slope_0 * 2
	idx = 0*seqLen*seqLen + 0*seqLen + 2
	expected = -slope0 * 2
	if math.Abs(float64(biasData[idx]-expected)) > 1e-5 {
		t.Errorf("bias[0, 0, 2] = %f, want %f", biasData[idx], expected)
	}
}

func TestALiBiInvalidSeqLen(t *testing.T) {
	backend := cpu.New()
	alibi := NewALiBi(4, backend)

	defer func() {
		if r := recover(); r == nil {
			t.Error("GetBias() with seqLen <= 0 should panic")
		}
	}()

	_ = alibi.GetBias(0)
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkSinusoidalPositionalEncodingForward(b *testing.B) {
	backend := cpu.New()
	pe := NewSinusoidalPositionalEncoding(2048, 512, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pe.Forward(128)
	}
}

func BenchmarkLearnedPositionalEmbeddingForward(b *testing.B) {
	backend := cpu.New()
	pe := NewLearnedPositionalEmbedding(2048, 512, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pe.Forward(128)
	}
}

func BenchmarkALiBiGetBias(b *testing.B) {
	backend := cpu.New()
	alibi := NewALiBi(12, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = alibi.GetBias(128)
	}
}

func BenchmarkALiBiGetBiasLarge(b *testing.B) {
	backend := cpu.New()
	alibi := NewALiBi(16, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = alibi.GetBias(512)
	}
}
