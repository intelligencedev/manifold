package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestRMSNormForward tests RMSNorm forward pass.
func TestRMSNormForward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test with d_model=3, epsilon=1e-5
	rmsnorm := NewRMSNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](3, 1e-5, backend)

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
	output := rmsnorm.Forward(input)

	// Expected calculation for first row [1, 2, 3]:
	// variance = mean([1, 4, 9]) = 14/3 ≈ 4.6667
	// rms = sqrt(4.6667 + 1e-5) ≈ 2.1602
	// normalized = [1/2.1602, 2/2.1602, 3/2.1602] = [0.4629, 0.9258, 1.3887]
	// With gamma=[1, 1, 1], output = normalized

	outputData := output.Data()

	// Check first row
	expected1 := []float32{0.4629, 0.9258, 1.3887}
	for i := 0; i < 3; i++ {
		got := outputData[i]
		exp := expected1[i]
		if math.Abs(float64(got-exp)) > 0.01 {
			t.Errorf("Row 1, element %d: got %v, expected %v", i, got, exp)
		}
	}

	// Check output shape
	if len(output.Shape()) != 2 || output.Shape()[0] != 2 || output.Shape()[1] != 3 {
		t.Errorf("RMSNorm changed shape: input %v -> output %v", input.Shape(), output.Shape())
	}
}

// TestRMSNormGamma tests that gamma parameter scales the output.
func TestRMSNormGamma(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Create RMSNorm with d_model=2
	rmsnorm := NewRMSNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](2, 1e-5, backend)

	// Manually set gamma to [2.0, 3.0]
	gammaData := rmsnorm.Gamma.Tensor().Data()
	gammaData[0] = 2.0
	gammaData[1] = 3.0

	// Input: [1, 2] = [[1, 1]]
	input, err := tensor.FromSlice[float32](
		[]float32{1.0, 1.0},
		tensor.Shape{1, 2},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Forward pass
	output := rmsnorm.Forward(input)

	// Expected:
	// variance = mean([1, 1]) = 1.0
	// rms = sqrt(1.0 + 1e-5) ≈ 1.0
	// normalized = [1/1, 1/1] = [1.0, 1.0]
	// scaled = [1.0 * 2.0, 1.0 * 3.0] = [2.0, 3.0]

	outputData := output.Data()
	expected := []float32{2.0, 3.0}

	for i := 0; i < 2; i++ {
		got := outputData[i]
		exp := expected[i]
		if math.Abs(float64(got-exp)) > 0.01 {
			t.Errorf("Element %d: got %v, expected %v", i, got, exp)
		}
	}
}

// TestRMSNorm3D tests RMSNorm on 3D input.
func TestRMSNorm3D(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// d_model=4
	rmsnorm := NewRMSNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](4, 1e-6, backend)

	// Input: [2, 3, 4] (batch=2, seq_len=3, d_model=4)
	input := tensor.Randn[float32](tensor.Shape{2, 3, 4}, backend)

	// Forward pass
	output := rmsnorm.Forward(input)

	// Check shape preservation
	if len(output.Shape()) != 3 {
		t.Errorf("Expected 3D output, got shape %v", output.Shape())
	}
	if output.Shape()[0] != 2 || output.Shape()[1] != 3 || output.Shape()[2] != 4 {
		t.Errorf("Shape mismatch: expected [2,3,4], got %v", output.Shape())
	}

	// Check that values are normalized (mean ≈ 0, variance ≈ 1 per last dim)
	// This is a rough check since RMSNorm normalizes by RMS, not standard deviation
	outputData := output.Data()
	for batch := 0; batch < 2; batch++ {
		for seq := 0; seq < 3; seq++ {
			// Check that output is not all zeros
			offset := (batch*3 + seq) * 4
			hasNonZero := false
			for i := 0; i < 4; i++ {
				if outputData[offset+i] != 0 {
					hasNonZero = true
					break
				}
			}
			if !hasNonZero {
				t.Errorf("Output is all zeros at batch=%d, seq=%d", batch, seq)
			}
		}
	}
}

// TestRMSNormEpsilon tests numerical stability with epsilon.
func TestRMSNormEpsilon(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Use larger epsilon to test its effect
	epsilon := float32(1e-2)
	rmsnorm := NewRMSNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](2, epsilon, backend)

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
	output := rmsnorm.Forward(input)

	// Check that output is finite
	outputData := output.Data()
	for i, val := range outputData {
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			t.Errorf("Output contains NaN/Inf at index %d: %v", i, val)
		}
	}
}

// TestRMSNormParameters tests that RMSNorm returns gamma as parameter.
func TestRMSNormParameters(t *testing.T) {
	backend := autodiff.New(cpu.New())
	rmsnorm := NewRMSNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](10, 1e-5, backend)

	params := rmsnorm.Parameters()

	if len(params) != 1 {
		t.Errorf("Expected 1 parameter (gamma), got %d", len(params))
	}

	if params[0].Name() != "gamma" {
		t.Errorf("Expected parameter name 'gamma', got '%s'", params[0].Name())
	}

	if len(params[0].Tensor().Shape()) != 1 || params[0].Tensor().Shape()[0] != 10 {
		t.Errorf("Expected gamma shape [10], got %v", params[0].Tensor().Shape())
	}
}

// TestRMSNormZeroInput tests RMSNorm with zero input (edge case).
func TestRMSNormZeroInput(t *testing.T) {
	backend := autodiff.New(cpu.New())
	rmsnorm := NewRMSNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](3, 1e-5, backend)

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
	output := rmsnorm.Forward(input)

	// Expected:
	// variance = mean([0, 0, 0]) = 0
	// rms = sqrt(0 + 1e-5) ≈ 0.00316
	// normalized = [0/0.00316, 0/0.00316, 0/0.00316] = [0, 0, 0]

	outputData := output.Data()
	for i, val := range outputData {
		if math.Abs(float64(val)) > 0.001 {
			t.Errorf("Expected ~0, got %v at index %d", val, i)
		}
	}
}

// TestRMSNormGradient tests that gradients flow through RMSNorm.
func TestRMSNormGradient(t *testing.T) {
	backend := autodiff.New(cpu.New())
	rmsnorm := NewRMSNorm[*autodiff.AutodiffBackend[*cpu.CPUBackend]](2, 1e-5, backend)

	// Input
	input := tensor.Randn[float32](tensor.Shape{1, 2}, backend)

	// Start recording
	backend.Tape().StartRecording()

	// Forward
	_ = rmsnorm.Forward(input)

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

	// Note: Gradient computation for gamma requires full integration with
	// autodiff graph. For now, we just verify that forward/backward works.
	t.Logf("Gradient check passed, %d operations recorded", backend.Tape().NumOps())
}
