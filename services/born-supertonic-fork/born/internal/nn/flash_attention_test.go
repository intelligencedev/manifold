package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestOnlineSoftmax tests online softmax vs standard softmax.
func TestOnlineSoftmax(t *testing.T) {
	headDim := 4
	scores := []float32{1.0, 2.0, 3.0}
	values := []float32{
		1, 0, 0, 0, // v0
		0, 1, 0, 0, // v1
		0, 0, 1, 0, // v2
	}

	// Compute using OnlineSoftmax
	os := NewOnlineSoftmax(headDim)
	os.Update(scores, values)
	result := os.Normalize()

	// Compute using standard softmax
	weights := attentionSoftmax(scores)
	expected := make([]float32, headDim)
	for i := 0; i < len(scores); i++ {
		for d := 0; d < headDim; d++ {
			expected[d] += weights[i] * values[i*headDim+d]
		}
	}

	// Compare
	for d := 0; d < headDim; d++ {
		if math.Abs(float64(result[d]-expected[d])) > 1e-5 {
			t.Errorf("Dimension %d: OnlineSoftmax = %v, expected %v", d, result[d], expected[d])
		}
	}
}

// TestOnlineSoftmaxMultipleBlocks tests incremental updates across multiple blocks.
func TestOnlineSoftmaxMultipleBlocks(t *testing.T) {
	headDim := 3

	// Block 1: 2 elements
	scores1 := []float32{1.0, 2.0}
	values1 := []float32{
		1, 0, 0, // v0
		0, 1, 0, // v1
	}

	// Block 2: 2 elements
	scores2 := []float32{3.0, 0.5}
	values2 := []float32{
		0, 0, 1, // v2
		0.5, 0.5, 0.5, // v3
	}

	// Compute using OnlineSoftmax (incremental)
	os := NewOnlineSoftmax(headDim)
	os.Update(scores1, values1)
	os.Update(scores2, values2)
	result := os.Normalize()

	// Compute using standard softmax (all at once)
	allScores := scores1
	allScores = append(allScores, scores2...)
	allValues := values1
	allValues = append(allValues, values2...)
	weights := attentionSoftmax(allScores)

	expected := make([]float32, headDim)
	for i := 0; i < len(allScores); i++ {
		for d := 0; d < headDim; d++ {
			expected[d] += weights[i] * allValues[i*headDim+d]
		}
	}

	// Compare
	for d := 0; d < headDim; d++ {
		if math.Abs(float64(result[d]-expected[d])) > 1e-5 {
			t.Errorf("Dimension %d: OnlineSoftmax = %v, expected %v", d, result[d], expected[d])
		}
	}
}

// TestOnlineSoftmaxReset tests the Reset functionality.
func TestOnlineSoftmaxReset(t *testing.T) {
	headDim := 2
	scores := []float32{1.0, 2.0}
	values := []float32{1, 0, 0, 1}

	os := NewOnlineSoftmax(headDim)

	// First use
	os.Update(scores, values)
	result1 := os.Normalize()

	// Reset and reuse
	os.Reset()
	os.Update(scores, values)
	result2 := os.Normalize()

	// Should get identical results
	for d := 0; d < headDim; d++ {
		if math.Abs(float64(result1[d]-result2[d])) > 1e-7 {
			t.Errorf("After reset: dimension %d differs: %v vs %v", d, result1[d], result2[d])
		}
	}
}

// TestFlashAttentionVsStandard tests Flash Attention correctness against standard attention.
func TestFlashAttentionVsStandard(t *testing.T) {
	backend := cpu.New()

	batch := 2
	seqLen := 16
	kvLen := 16
	numHeads := 4
	headDim := 8
	blockSize := 4

	// Create random Q, K, V
	Q, err := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create Q: %v", err)
	}

	K, err := tensor.FromSlice[float32](
		randomFloat32(batch*kvLen*numHeads*headDim),
		tensor.Shape{batch, kvLen, numHeads, headDim},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create K: %v", err)
	}

	V, err := tensor.FromSlice[float32](
		randomFloat32(batch*kvLen*numHeads*headDim),
		tensor.Shape{batch, kvLen, numHeads, headDim},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create V: %v", err)
	}

	// Compute using Flash Attention
	config := FlashAttentionConfig{
		NumHeads:   numHeads,
		HeadDim:    headDim,
		MaxSeqLen:  seqLen,
		CausalMask: false,
		BlockSize:  blockSize,
	}
	fa := NewFlashAttention[float32](config, backend)
	flashOutput := fa.Forward(Q, K, V, nil)

	// Compute using Standard Attention
	scale := float32(1.0 / math.Sqrt(float64(headDim)))
	standardOutput := StandardAttention(
		Q.Data(), K.Data(), V.Data(),
		batch, seqLen, kvLen, numHeads, headDim,
		scale, false,
	)

	// Compare outputs
	flashData := flashOutput.Data()
	maxError := float32(0)
	for i := range flashData {
		err := float32(math.Abs(float64(flashData[i] - standardOutput[i])))
		if err > maxError {
			maxError = err
		}
	}

	// Error threshold: 1e-4 (allowing for floating point differences)
	if maxError > 1e-4 {
		t.Errorf("Flash Attention vs Standard Attention: max error = %v, expected < 1e-4", maxError)
	}
}

// TestFlashAttentionCausal tests Flash Attention with causal masking.
func TestFlashAttentionCausal(t *testing.T) {
	backend := cpu.New()

	batch := 1
	seqLen := 8
	numHeads := 2
	headDim := 4
	blockSize := 4

	// Create random Q, K, V
	Q, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	K, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	V, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)

	// Flash Attention with causal mask
	config := FlashAttentionConfig{
		NumHeads:   numHeads,
		HeadDim:    headDim,
		MaxSeqLen:  seqLen,
		CausalMask: true,
		BlockSize:  blockSize,
	}
	fa := NewFlashAttention[float32](config, backend)
	flashOutput := fa.Forward(Q, K, V, nil)

	// Standard Attention with causal mask
	scale := float32(1.0 / math.Sqrt(float64(headDim)))
	standardOutput := StandardAttention(
		Q.Data(), K.Data(), V.Data(),
		batch, seqLen, seqLen, numHeads, headDim,
		scale, true,
	)

	// Compare
	flashData := flashOutput.Data()
	maxError := float32(0)
	for i := range flashData {
		err := float32(math.Abs(float64(flashData[i] - standardOutput[i])))
		if err > maxError {
			maxError = err
		}
	}

	if maxError > 1e-4 {
		t.Errorf("Causal Flash Attention: max error = %v, expected < 1e-4", maxError)
	}
}

// TestFlashAttentionBatched tests batched processing.
func TestFlashAttentionBatched(t *testing.T) {
	backend := cpu.New()

	batch := 4
	seqLen := 12
	numHeads := 3
	headDim := 16
	blockSize := 6

	Q, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	K, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	V, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)

	config := FlashAttentionConfig{
		NumHeads:   numHeads,
		HeadDim:    headDim,
		MaxSeqLen:  seqLen,
		CausalMask: false,
		BlockSize:  blockSize,
	}
	fa := NewFlashAttention[float32](config, backend)
	flashOutput := fa.Forward(Q, K, V, nil)

	// Verify output shape
	expectedShape := tensor.Shape{batch, seqLen, numHeads, headDim}
	if !shapeEqual(flashOutput.Shape(), expectedShape) {
		t.Errorf("Output shape = %v, expected %v", flashOutput.Shape(), expectedShape)
	}

	// Verify output is reasonable (not NaN or Inf)
	data := flashOutput.Data()
	for i, val := range data {
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			t.Errorf("Output[%d] = %v, expected finite value", i, val)
			break
		}
	}
}

// TestFlashAttentionSmallBlockSize tests with block size smaller than sequence.
func TestFlashAttentionSmallBlockSize(t *testing.T) {
	backend := cpu.New()

	batch := 1
	seqLen := 10
	numHeads := 2
	headDim := 4
	blockSize := 3 // Not evenly divisible into seqLen

	Q, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	K, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	V, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)

	config := FlashAttentionConfig{
		NumHeads:   numHeads,
		HeadDim:    headDim,
		MaxSeqLen:  seqLen,
		CausalMask: false,
		BlockSize:  blockSize,
	}
	fa := NewFlashAttention[float32](config, backend)
	flashOutput := fa.Forward(Q, K, V, nil)

	// Compare with standard attention
	scale := float32(1.0 / math.Sqrt(float64(headDim)))
	standardOutput := StandardAttention(
		Q.Data(), K.Data(), V.Data(),
		batch, seqLen, seqLen, numHeads, headDim,
		scale, false,
	)

	flashData := flashOutput.Data()
	maxError := float32(0)
	for i := range flashData {
		err := float32(math.Abs(float64(flashData[i] - standardOutput[i])))
		if err > maxError {
			maxError = err
		}
	}

	if maxError > 1e-4 {
		t.Errorf("Small block size: max error = %v, expected < 1e-4", maxError)
	}
}

// BenchmarkFlashAttentionVsStandard benchmarks Flash Attention vs Standard Attention.
func BenchmarkFlashAttentionVsStandard(b *testing.B) {
	backend := cpu.New()

	batch := 8
	seqLen := 128
	numHeads := 8
	headDim := 64
	blockSize := 64

	Q, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	K, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	V, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)

	config := FlashAttentionConfig{
		NumHeads:   numHeads,
		HeadDim:    headDim,
		MaxSeqLen:  seqLen,
		CausalMask: false,
		BlockSize:  blockSize,
	}
	fa := NewFlashAttention[float32](config, backend)

	scale := float32(1.0 / math.Sqrt(float64(headDim)))

	b.Run("FlashAttention", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fa.Forward(Q, K, V, nil)
		}
	})

	b.Run("StandardAttention", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			StandardAttention(
				Q.Data(), K.Data(), V.Data(),
				batch, seqLen, seqLen, numHeads, headDim,
				scale, false,
			)
		}
	})
}

// BenchmarkFlashAttentionCausal benchmarks causal Flash Attention.
func BenchmarkFlashAttentionCausal(b *testing.B) {
	backend := cpu.New()

	batch := 4
	seqLen := 256
	numHeads := 12
	headDim := 64
	blockSize := 64

	Q, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	K, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)
	V, _ := tensor.FromSlice[float32](
		randomFloat32(batch*seqLen*numHeads*headDim),
		tensor.Shape{batch, seqLen, numHeads, headDim},
		backend,
	)

	config := FlashAttentionConfig{
		NumHeads:   numHeads,
		HeadDim:    headDim,
		MaxSeqLen:  seqLen,
		CausalMask: true,
		BlockSize:  blockSize,
	}
	fa := NewFlashAttention[float32](config, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fa.Forward(Q, K, V, nil)
	}
}

// Helper function to generate random float32 slice.
func randomFloat32(size int) []float32 {
	data := make([]float32, size)
	for i := range data {
		data[i] = float32(i%100) / 100.0 // Simple deterministic pattern for tests
	}
	return data
}

// shapeEqual checks if two shapes are equal.
