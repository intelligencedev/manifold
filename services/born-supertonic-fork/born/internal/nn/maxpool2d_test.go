package nn

import (
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestMaxPool2D_Creation tests MaxPool2D layer creation.
func TestMaxPool2D_Creation(t *testing.T) {
	backend := cpu.New()

	// Create MaxPool2D: 2x2 kernel, stride=2
	pool := NewMaxPool2D(2, 2, backend)

	if pool.KernelSize() != 2 {
		t.Errorf("Expected kernel_size=2, got %d", pool.KernelSize())
	}
	if pool.Stride() != 2 {
		t.Errorf("Expected stride=2, got %d", pool.Stride())
	}

	// Check parameters (should be empty)
	params := pool.Parameters()
	if len(params) != 0 {
		t.Errorf("Expected 0 parameters (MaxPool2D has no learnable params), got %d", len(params))
	}
}

// TestMaxPool2D_ForwardShape tests forward pass output shape.
func TestMaxPool2D_ForwardShape(t *testing.T) {
	backend := cpu.New()

	// MaxPool2D: 2x2 kernel, stride=2
	pool := NewMaxPool2D(2, 2, backend)

	// Input: [2, 3, 28, 28]
	input := tensor.Zeros[float32](tensor.Shape{2, 3, 28, 28}, backend)

	// Forward
	output := pool.Forward(input)

	// Output shape should be [2, 3, 14, 14]
	// out_h = (28 - 2) / 2 + 1 = 14
	expectedShape := tensor.Shape{2, 3, 14, 14}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape: expected %v, got %v", expectedShape, output.Shape())
	}
}

// TestMaxPool2D_ForwardValues tests forward pass with known values.
func TestMaxPool2D_ForwardValues(t *testing.T) {
	backend := cpu.New()

	// Create 2x2 max pooling
	pool := NewMaxPool2D(2, 2, backend)

	// Input: [1, 1, 4, 4] with sequential values 1-16
	input := tensor.Zeros[float32](tensor.Shape{1, 1, 4, 4}, backend)
	inputData := input.Raw().AsFloat32()
	for i := 0; i < 16; i++ {
		inputData[i] = float32(i + 1)
	}

	// Forward
	output := pool.Forward(input)

	// Expected output (max in each 2x2 window):
	// [[1,2,3,4],      -> [[6,8],
	//  [5,6,7,8],         [14,16]]
	//  [9,10,11,12],
	//  [13,14,15,16]]
	expected := []float32{6, 8, 14, 16}
	outputData := output.Raw().AsFloat32()

	for i, exp := range expected {
		if outputData[i] != exp {
			t.Errorf("Output[%d]: expected %.1f, got %.1f", i, exp, outputData[i])
		}
	}
}

// TestMaxPool2D_WithDifferentStride tests pooling with stride != kernelSize.
func TestMaxPool2D_WithDifferentStride(t *testing.T) {
	backend := cpu.New()

	// Create 3x3 pooling with stride=2 (overlapping)
	pool := NewMaxPool2D(3, 2, backend)

	// Input: [1, 1, 7, 7]
	input := tensor.Ones[float32](tensor.Shape{1, 1, 7, 7}, backend)

	// Forward
	output := pool.Forward(input)

	// Output shape: (7 - 3) / 2 + 1 = 3
	expectedShape := tensor.Shape{1, 1, 3, 3}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape: expected %v, got %v", expectedShape, output.Shape())
	}

	// All values should be 1.0 (max of ones)
	outputData := output.Raw().AsFloat32()
	for i, val := range outputData {
		if val != 1.0 {
			t.Errorf("Output[%d]: expected 1.0, got %.1f", i, val)
		}
	}
}

// TestMaxPool2D_ComputeOutputSize tests output size computation.
func TestMaxPool2D_ComputeOutputSize(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		kernelSize, stride   int
		inputH, inputW       int
		expectedH, expectedW int
	}{
		{2, 2, 28, 28, 14, 14}, // Standard 2x2 pooling
		{2, 2, 32, 32, 16, 16}, // ImageNet-style input
		{3, 2, 28, 28, 13, 13}, // Overlapping pooling
		{2, 1, 5, 5, 4, 4},     // Stride 1 (heavy overlap)
	}

	for _, tt := range tests {
		pool := NewMaxPool2D(tt.kernelSize, tt.stride, backend)
		outSize := pool.ComputeOutputSize(tt.inputH, tt.inputW)

		if outSize[0] != tt.expectedH || outSize[1] != tt.expectedW {
			t.Errorf("ComputeOutputSize(kernel=%d, stride=%d, input=%dx%d): expected [%d,%d], got %v",
				tt.kernelSize, tt.stride, tt.inputH, tt.inputW,
				tt.expectedH, tt.expectedW, outSize)
		}
	}
}

// TestMaxPool2D_IntegrationWithAutodiff tests MaxPool2D with autodiff backend.
func TestMaxPool2D_IntegrationWithAutodiff(t *testing.T) {
	cpuBackend := cpu.New()
	backend := autodiff.New(cpuBackend)

	// Start recording
	backend.Tape().StartRecording()

	// Create MaxPool2D layer
	pool := NewMaxPool2D(2, 2, backend)

	// Input: [1, 2, 4, 4]
	input := tensor.Randn[float32](tensor.Shape{1, 2, 4, 4}, backend)

	// Forward
	output := pool.Forward(input)

	// Check output shape: [1, 2, 2, 2]
	expectedShape := tensor.Shape{1, 2, 2, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape: expected %v, got %v", expectedShape, output.Shape())
	}

	// Create output gradient (all ones)
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, backend.Device())
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward pass
	grads := backend.Tape().Backward(outputGrad, backend)

	// Check that input gradient exists
	inputGrad, hasInputGrad := grads[input.Raw()]

	if !hasInputGrad {
		t.Fatal("No gradient for input!")
	}

	// Verify gradient shape matches input
	if !inputGrad.Shape().Equal(input.Shape()) {
		t.Errorf("Input gradient shape: expected %v, got %v", input.Shape(), inputGrad.Shape())
	}

	// Verify gradients are non-zero (should have 4 non-zero per channel)
	inputGradData := inputGrad.AsFloat32()
	nonZeroCount := 0
	for _, g := range inputGradData {
		if g != 0.0 {
			nonZeroCount++
		}
	}

	// Should have 8 non-zero gradients (4 max positions per channel * 2 channels)
	if nonZeroCount != 8 {
		t.Errorf("Expected 8 non-zero gradients, got %d", nonZeroCount)
	}

	t.Log("SUCCESS: MaxPool2D integrates correctly with autodiff")
	t.Logf("  Input gradient: %d/%d non-zero (gradient routing to max positions)", nonZeroCount, len(inputGradData))
}

// TestMaxPool2D_AfterConv2D tests typical CNN pattern: Conv2D -> MaxPool2D.
func TestMaxPool2D_AfterConv2D(t *testing.T) {
	cpuBackend := cpu.New()
	backend := autodiff.New(cpuBackend)

	// Start recording
	backend.Tape().StartRecording()

	// Create Conv2D -> MaxPool2D layers (typical CNN pattern)
	conv := NewConv2D(1, 6, 5, 5, 1, 0, true, backend) // 1->6 channels, 5x5 kernel
	pool := NewMaxPool2D(2, 2, backend)                // 2x2 pooling

	// Input: [2, 1, 28, 28] (like MNIST)
	input := tensor.Randn[float32](tensor.Shape{2, 1, 28, 28}, backend)

	// Forward: Conv2D then MaxPool2D
	convOut := conv.Forward(input)   // [2, 6, 24, 24]
	poolOut := pool.Forward(convOut) // [2, 6, 12, 12]

	// Verify final shape
	expectedShape := tensor.Shape{2, 6, 12, 12}
	if !poolOut.Shape().Equal(expectedShape) {
		t.Errorf("Final output shape: expected %v, got %v", expectedShape, poolOut.Shape())
	}

	// Create output gradient
	outputGrad, _ := tensor.NewRaw(poolOut.Shape(), tensor.Float32, backend.Device())
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward pass
	grads := backend.Tape().Backward(outputGrad, backend)

	// Verify gradients exist for Conv2D parameters
	_, hasWeightGrad := grads[conv.weight.Tensor().Raw()]
	_, hasBiasGrad := grads[conv.bias.Tensor().Raw()]
	_, hasInputGrad := grads[input.Raw()]

	if !hasWeightGrad {
		t.Error("No gradient for Conv2D weight!")
	}
	if !hasBiasGrad {
		t.Error("No gradient for Conv2D bias!")
	}
	if !hasInputGrad {
		t.Error("No gradient for input!")
	}

	t.Log("SUCCESS: Conv2D -> MaxPool2D gradient flow correct")
}
