//go:build windows || linux

package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/webgpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestFlashAttentionGPU tests GPU vs CPU correctness.
func TestFlashAttentionGPU(t *testing.T) {
	// Try to create WebGPU backend (skip test if not available)
	gpuBackend, err := webgpu.New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer gpuBackend.Release()

	tests := []struct {
		name      string
		batch     int
		seqLen    int
		kvLen     int
		numHeads  int
		headDim   int
		causal    bool
		blockSize int
		maxError  float64
	}{
		{
			name:      "small_non_causal",
			batch:     2,
			seqLen:    32,
			kvLen:     32,
			numHeads:  4,
			headDim:   64,
			causal:    false,
			blockSize: 64,
			maxError:  1e-4,
		},
		{
			name:      "small_causal",
			batch:     2,
			seqLen:    32,
			kvLen:     32,
			numHeads:  4,
			headDim:   64,
			causal:    true,
			blockSize: 64,
			maxError:  1e-4,
		},
		{
			name:      "medium_head_dim_128",
			batch:     1,
			seqLen:    64,
			kvLen:     64,
			numHeads:  8,
			headDim:   128,
			causal:    false,
			blockSize: 64,
			maxError:  1e-4,
		},
		{
			name:      "cross_attention",
			batch:     2,
			seqLen:    32,
			kvLen:     48,
			numHeads:  4,
			headDim:   64,
			causal:    false,
			blockSize: 64,
			maxError:  1e-4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data
			qData := makeRandomData(tt.batch * tt.seqLen * tt.numHeads * tt.headDim)
			kData := makeRandomData(tt.batch * tt.kvLen * tt.numHeads * tt.headDim)
			vData := makeRandomData(tt.batch * tt.kvLen * tt.numHeads * tt.headDim)

			shape := tensor.Shape{tt.batch, tt.seqLen, tt.numHeads, tt.headDim}
			kvShape := tensor.Shape{tt.batch, tt.kvLen, tt.numHeads, tt.headDim}

			// GPU computation
			qGPU, _ := tensor.FromSlice(qData, shape, gpuBackend)
			kGPU, _ := tensor.FromSlice(kData, kvShape, gpuBackend)
			vGPU, _ := tensor.FromSlice(vData, kvShape, gpuBackend)

			configGPU := FlashAttentionConfig{
				NumHeads:   tt.numHeads,
				HeadDim:    tt.headDim,
				MaxSeqLen:  tt.seqLen,
				CausalMask: tt.causal,
				BlockSize:  tt.blockSize,
			}
			faGPU := NewFlashAttention[float32](configGPU, gpuBackend)
			outputGPU := faGPU.Forward(qGPU, kGPU, vGPU, nil)
			gpuResult := outputGPU.Data()

			// CPU reference computation
			scale := float32(1.0 / math.Sqrt(float64(tt.headDim)))
			cpuResult := flashAttentionCPU(
				qData, kData, vData,
				tt.batch, tt.seqLen, tt.kvLen, tt.numHeads, tt.headDim,
				scale,
				tt.causal,
				tt.blockSize,
			)

			// Compare results
			if len(gpuResult) != len(cpuResult) {
				t.Fatalf("Output size mismatch: GPU=%d, CPU=%d", len(gpuResult), len(cpuResult))
			}

			maxDiff := float32(0.0)
			for i := range gpuResult {
				diff := absFloat32(gpuResult[i] - cpuResult[i])
				if diff > maxDiff {
					maxDiff = diff
				}
			}

			if float64(maxDiff) > tt.maxError {
				t.Errorf("Max difference %.6f exceeds threshold %.6f", maxDiff, tt.maxError)
			}
		})
	}
}

// TestFlashAttentionGPUHeadDimensions tests various head dimensions.
func TestFlashAttentionGPUHeadDimensions(t *testing.T) {
	gpuBackend, err := webgpu.New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer gpuBackend.Release()

	headDims := []int{64, 96, 128, 256}

	for _, headDim := range headDims {
		t.Run("head_dim_"+string(rune('0'+headDim/10)), func(t *testing.T) {
			batch := 1
			seqLen := 32
			numHeads := 4

			qData := makeRandomData(batch * seqLen * numHeads * headDim)
			kData := makeRandomData(batch * seqLen * numHeads * headDim)
			vData := makeRandomData(batch * seqLen * numHeads * headDim)

			shape := tensor.Shape{batch, seqLen, numHeads, headDim}

			qGPU, _ := tensor.FromSlice(qData, shape, gpuBackend)
			kGPU, _ := tensor.FromSlice(kData, shape, gpuBackend)
			vGPU, _ := tensor.FromSlice(vData, shape, gpuBackend)

			config := FlashAttentionConfig{
				NumHeads:   numHeads,
				HeadDim:    headDim,
				MaxSeqLen:  seqLen,
				CausalMask: false,
				BlockSize:  64,
			}
			fa := NewFlashAttention[float32](config, gpuBackend)
			output := fa.Forward(qGPU, kGPU, vGPU, nil)

			// Verify output shape
			if !output.Shape().Equal(shape) {
				t.Errorf("Output shape mismatch: got %v, want %v", output.Shape(), shape)
			}

			// Verify no NaN or Inf values
			result := output.Data()
			for i, v := range result {
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					t.Errorf("Invalid value at index %d: %f", i, v)
					break
				}
			}
		})
	}
}

// TestFlashAttentionGPUBlockSizes tests different tile sizes.
func TestFlashAttentionGPUBlockSizes(t *testing.T) {
	gpuBackend, err := webgpu.New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer gpuBackend.Release()

	blockSizes := []int{64, 128}

	for _, blockSize := range blockSizes {
		t.Run("block_size_"+string(rune('0'+blockSize/10)), func(t *testing.T) {
			batch := 2
			seqLen := 128
			numHeads := 8
			headDim := 64

			qData := makeRandomData(batch * seqLen * numHeads * headDim)
			kData := makeRandomData(batch * seqLen * numHeads * headDim)
			vData := makeRandomData(batch * seqLen * numHeads * headDim)

			shape := tensor.Shape{batch, seqLen, numHeads, headDim}

			qGPU, _ := tensor.FromSlice(qData, shape, gpuBackend)
			kGPU, _ := tensor.FromSlice(kData, shape, gpuBackend)
			vGPU, _ := tensor.FromSlice(vData, shape, gpuBackend)

			config := FlashAttentionConfig{
				NumHeads:   numHeads,
				HeadDim:    headDim,
				MaxSeqLen:  seqLen,
				CausalMask: false,
				BlockSize:  blockSize,
			}
			fa := NewFlashAttention[float32](config, gpuBackend)
			output := fa.Forward(qGPU, kGPU, vGPU, nil)

			// Verify output shape
			if !output.Shape().Equal(shape) {
				t.Errorf("Output shape mismatch: got %v, want %v", output.Shape(), shape)
			}
		})
	}
}

// Helper functions.

func makeRandomData(size int) []float32 {
	data := make([]float32, size)
	// Simple deterministic pseudo-random for reproducibility
	for i := range data {
		data[i] = float32((i*7+13)%100-50) / 100.0
	}
	return data
}

func absFloat32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
