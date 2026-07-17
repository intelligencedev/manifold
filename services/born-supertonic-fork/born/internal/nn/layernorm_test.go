package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestLayerNorm_Basic tests LayerNorm forward pass with basic input.
func TestLayerNorm_Basic(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test with normalized_shape=3, epsilon=1e-5
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](3, 1e-5, backend)

	// Input: [2, 3] = [[1, 2, 3], [4, 5, 6]]
	input, err := tensor.FromSlice[float32](
		[]float32{1.0, 2.0, 3.0, 4.0, 5.0, 6.0},
		tensor.Shape{2, 3},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Forward pass
	output := layernorm.Forward(input)

	// Expected calculation for first row [1, 2, 3]:
	// mean = (1 + 2 + 3) / 3 = 2.0
	// x_centered = [-1, 0, 1]
	// variance = (1 + 0 + 1) / 3 = 0.6667
	// std = sqrt(0.6667 + 1e-5) ≈ 0.8165
	// normalized = [-1/0.8165, 0/0.8165, 1/0.8165] = [-1.2247, 0, 1.2247]
	// With gamma=[1, 1, 1], beta=[0, 0, 0], output = normalized

	outputData := output.Data()

	// Check first row
	expected1 := []float32{-1.2247, 0.0, 1.2247}
	for i := 0; i < 3; i++ {
		got := outputData[i]
		exp := expected1[i]
		if math.Abs(float64(got-exp)) > 0.01 {
			t.Errorf("Row 1, element %d: got %v, expected %v", i, got, exp)
		}
	}

	// Check output shape
	if len(output.Shape()) != 2 || output.Shape()[0] != 2 || output.Shape()[1] != 3 {
		t.Errorf("LayerNorm changed shape: input %v -> output %v", input.Shape(), output.Shape())
	}
}

// TestLayerNorm_GammaAndBeta tests that gamma and beta parameters work correctly.
func TestLayerNorm_GammaAndBeta(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create LayerNorm with normalized_shape=2
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](2, 1e-5, backend)

	// Manually set gamma to [2.0, 3.0] and beta to [0.5, 1.0]
	gammaData := layernorm.Gamma.Tensor().Data()
	gammaData[0] = 2.0
	gammaData[1] = 3.0

	betaData := layernorm.Beta.Tensor().Data()
	betaData[0] = 0.5
	betaData[1] = 1.0

	// Input: [1, 2] = [[2, 4]]
	input, err := tensor.FromSlice[float32](
		[]float32{2.0, 4.0},
		tensor.Shape{1, 2},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Forward pass
	output := layernorm.Forward(input)

	// Expected:
	// mean = (2 + 4) / 2 = 3.0
	// x_centered = [-1, 1]
	// variance = (1 + 1) / 2 = 1.0
	// std = sqrt(1.0 + 1e-5) ≈ 1.0
	// normalized = [-1.0, 1.0]
	// scaled = [-1.0 * 2.0 + 0.5, 1.0 * 3.0 + 1.0] = [-1.5, 4.0]

	outputData := output.Data()
	expected := []float32{-1.5, 4.0}

	for i := 0; i < 2; i++ {
		got := outputData[i]
		exp := expected[i]
		if math.Abs(float64(got-exp)) > 0.01 {
			t.Errorf("Element %d: got %v, expected %v", i, got, exp)
		}
	}
}

// TestLayerNorm_3D tests LayerNorm on 3D input (batch, seq, features).
func TestLayerNorm_3D(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// normalized_shape=4
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](4, 1e-6, backend)

	// Input: [2, 3, 4] (batch=2, seq_len=3, d_model=4)
	input := tensor.Randn[float32](tensor.Shape{2, 3, 4}, backend)

	// Forward pass
	output := layernorm.Forward(input)

	// Check shape preservation
	if len(output.Shape()) != 3 {
		t.Errorf("Expected 3D output, got shape %v", output.Shape())
	}
	if output.Shape()[0] != 2 || output.Shape()[1] != 3 || output.Shape()[2] != 4 {
		t.Errorf("Shape mismatch: expected [2,3,4], got %v", output.Shape())
	}

	// Check that values are normalized
	outputData := output.Data()
	for batch := 0; batch < 2; batch++ {
		for seq := 0; seq < 3; seq++ {
			// For each position, compute mean and variance of the normalized output
			offset := (batch*3 + seq) * 4
			var sum, sumSq float64
			for i := 0; i < 4; i++ {
				val := float64(outputData[offset+i])
				sum += val
				sumSq += val * val
			}
			mean := sum / 4.0
			variance := sumSq/4.0 - mean*mean

			// After normalization, mean should be ≈ 0, variance ≈ 1
			// With learnable gamma and beta (initialized to 1 and 0), this should hold
			if math.Abs(mean) > 0.01 {
				t.Errorf("Mean not normalized at batch=%d, seq=%d: got %v, expected ~0", batch, seq, mean)
			}
			if math.Abs(variance-1.0) > 0.1 {
				t.Errorf("Variance not normalized at batch=%d, seq=%d: got %v, expected ~1", batch, seq, variance)
			}
		}
	}
}

// TestLayerNorm_4D tests LayerNorm on 4D input.
func TestLayerNorm_4D(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// normalized_shape=8
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](8, 1e-5, backend)

	// Input: [2, 4, 3, 8] (batch=2, channels=4, height=3, features=8)
	input := tensor.Randn[float32](tensor.Shape{2, 4, 3, 8}, backend)

	// Forward pass
	output := layernorm.Forward(input)

	// Check shape preservation
	expectedShape := tensor.Shape{2, 4, 3, 8}
	if len(output.Shape()) != 4 {
		t.Errorf("Expected 4D output, got shape %v", output.Shape())
	}
	for i := 0; i < 4; i++ {
		if output.Shape()[i] != expectedShape[i] {
			t.Errorf("Shape mismatch at dim %d: expected %d, got %d", i, expectedShape[i], output.Shape()[i])
		}
	}

	// Check that output is not all zeros
	outputData := output.Data()
	hasNonZero := false
	for _, val := range outputData {
		if val != 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("Output is all zeros")
	}
}

// TestLayerNorm_Parameters tests that LayerNorm returns gamma and beta as parameters.
func TestLayerNorm_Parameters(t *testing.T) {
	backend := autodiff.New(cpu.New())
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](10, 1e-5, backend)

	params := layernorm.Parameters()

	if len(params) != 2 {
		t.Errorf("Expected 2 parameters (gamma and beta), got %d", len(params))
	}

	// Check gamma
	if params[0].Name() != "gamma" {
		t.Errorf("Expected first parameter name 'gamma', got '%s'", params[0].Name())
	}
	if len(params[0].Tensor().Shape()) != 1 || params[0].Tensor().Shape()[0] != 10 {
		t.Errorf("Expected gamma shape [10], got %v", params[0].Tensor().Shape())
	}

	// Check beta
	if params[1].Name() != "beta" {
		t.Errorf("Expected second parameter name 'beta', got '%s'", params[1].Name())
	}
	if len(params[1].Tensor().Shape()) != 1 || params[1].Tensor().Shape()[0] != 10 {
		t.Errorf("Expected beta shape [10], got %v", params[1].Tensor().Shape())
	}
}

// TestLayerNorm_NumericalStability tests numerical stability with epsilon.
func TestLayerNorm_NumericalStability(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Use larger epsilon to test its effect
	epsilon := float32(1e-2)
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](2, epsilon, backend)

	// Input with very small values (close to zero)
	input, err := tensor.FromSlice[float32](
		[]float32{1e-8, 1e-8},
		tensor.Shape{1, 2},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Forward pass should not panic or produce NaN/Inf
	output := layernorm.Forward(input)

	// Check that output is finite
	outputData := output.Data()
	for i, val := range outputData {
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			t.Errorf("Output contains NaN/Inf at index %d: %v", i, val)
		}
	}
}

// TestLayerNorm_ZeroInput tests LayerNorm with zero input (edge case).
func TestLayerNorm_ZeroInput(t *testing.T) {
	backend := autodiff.New(cpu.New())
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](3, 1e-5, backend)

	// Input: all zeros
	input, err := tensor.FromSlice[float32](
		[]float32{0.0, 0.0, 0.0},
		tensor.Shape{1, 3},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Forward pass
	output := layernorm.Forward(input)

	// Expected:
	// mean = 0
	// x_centered = [0, 0, 0]
	// variance = 0
	// normalized = [0, 0, 0]
	// With beta = [0, 0, 0], output should be [0, 0, 0]

	outputData := output.Data()
	for i, val := range outputData {
		if math.Abs(float64(val)) > 0.001 {
			t.Errorf("Expected ~0, got %v at index %d", val, i)
		}
	}
}

// TestLayerNorm_Gradient tests that gradients flow through LayerNorm.
func TestLayerNorm_Gradient(t *testing.T) {
	backend := autodiff.New(cpu.New())
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](2, 1e-5, backend)

	// Input
	input := tensor.Randn[float32](tensor.Shape{1, 2}, backend)

	// Start recording
	backend.Tape().StartRecording()

	// Forward
	_ = layernorm.Forward(input)

	// Create output gradient
	outputGrad, err := tensor.FromSlice[float32](
		[]float32{1.0, 1.0},
		tensor.Shape{1, 2},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create output gradient: %v", err)
	}

	// Backward
	grads := backend.Tape().Backward(outputGrad.Raw(), backend)

	// Check that gradient exists for input
	_, hasInputGrad := grads[input.Raw()]
	if !hasInputGrad {
		t.Error("No gradient computed for input")
	}

	// Note: Gradient computation for gamma and beta requires full integration with
	// autodiff graph. For now, we just verify that forward/backward works.
	t.Logf("Gradient check passed, %d operations recorded", backend.Tape().NumOps())
}

// BenchmarkLayerNorm_768 benchmarks LayerNorm with d_model=768 (typical for transformers).
func BenchmarkLayerNorm_768(b *testing.B) {
	backend := autodiff.New(cpu.New())
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](768, 1e-5, backend)

	// Input: [32, 128, 768] (batch=32, seq_len=128, d_model=768)
	input := tensor.Randn[float32](tensor.Shape{32, 128, 768}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = layernorm.Forward(input)
	}
}

// BenchmarkLayerNorm_1024 benchmarks LayerNorm with d_model=1024.
func BenchmarkLayerNorm_1024(b *testing.B) {
	backend := autodiff.New(cpu.New())
	layernorm := NewLayerNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](1024, 1e-5, backend)

	// Input: [32, 128, 1024] (batch=32, seq_len=128, d_model=1024)
	input := tensor.Randn[float32](tensor.Shape{32, 128, 1024}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = layernorm.Forward(input)
	}
}
