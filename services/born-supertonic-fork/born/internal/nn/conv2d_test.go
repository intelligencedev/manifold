package nn

import (
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestConv2D_Creation tests Conv2D layer creation.
func TestConv2D_Creation(t *testing.T) {
	backend := cpu.New()

	// Create Conv2D: 1 -> 6 channels, 5x5 kernel
	conv := NewConv2D(1, 6, 5, 5, 1, 0, true, backend)

	if conv.InChannels() != 1 {
		t.Errorf("Expected in_channels=1, got %d", conv.InChannels())
	}
	if conv.OutChannels() != 6 {
		t.Errorf("Expected out_channels=6, got %d", conv.OutChannels())
	}

	kernelSize := conv.KernelSize()
	if kernelSize[0] != 5 || kernelSize[1] != 5 {
		t.Errorf("Expected kernel_size=[5,5], got %v", kernelSize)
	}

	// Check weight shape: [6, 1, 5, 5]
	weightShape := conv.weight.Tensor().Shape()
	expectedShape := tensor.Shape{6, 1, 5, 5}
	if !weightShape.Equal(expectedShape) {
		t.Errorf("Weight shape: expected %v, got %v", expectedShape, weightShape)
	}

	// Check bias shape: [6]
	biasShape := conv.bias.Tensor().Shape()
	expectedBiasShape := tensor.Shape{6}
	if !biasShape.Equal(expectedBiasShape) {
		t.Errorf("Bias shape: expected %v, got %v", expectedBiasShape, biasShape)
	}

	// Check parameters
	params := conv.Parameters()
	if len(params) != 2 {
		t.Errorf("Expected 2 parameters (weight, bias), got %d", len(params))
	}
}

// TestConv2D_ForwardShape tests forward pass output shape.
func TestConv2D_ForwardShape(t *testing.T) {
	backend := cpu.New()

	// Conv2D: 1 -> 6 channels, 5x5 kernel, stride=1, padding=0
	conv := NewConv2D(1, 6, 5, 5, 1, 0, true, backend)

	// Input: [2, 1, 28, 28] (like MNIST batch of 2)
	input := tensor.Zeros[float32](tensor.Shape{2, 1, 28, 28}, backend)

	// Forward
	output := conv.Forward(input)

	// Output shape should be [2, 6, 24, 24]
	// out_h = (28 + 2*0 - 5) / 1 + 1 = 24
	expectedShape := tensor.Shape{2, 6, 24, 24}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape: expected %v, got %v", expectedShape, output.Shape())
	}
}

// TestConv2D_ForwardValues tests forward pass with known values.
func TestConv2D_ForwardValues(t *testing.T) {
	backend := cpu.New()

	// Small test case: 1 -> 1 channel, 2x2 kernel
	conv := NewConv2D(1, 1, 2, 2, 1, 0, false, backend) // no bias

	// Set weight to known values
	weightData := conv.weight.Tensor().Raw().AsFloat32()
	weightData[0], weightData[1] = 1.0, 2.0
	weightData[2], weightData[3] = 3.0, 4.0

	// Input: [1, 1, 3, 3] with values 1-9
	input := tensor.Zeros[float32](tensor.Shape{1, 1, 3, 3}, backend)
	inputData := input.Raw().AsFloat32()
	for i := 0; i < 9; i++ {
		inputData[i] = float32(i + 1)
	}

	// Forward
	output := conv.Forward(input)

	// Expected output (manual computation):
	// [0,0]: 1*1 + 2*2 + 3*4 + 4*5 = 1+4+12+20 = 37
	// [0,1]: 1*2 + 2*3 + 3*5 + 4*6 = 2+6+15+24 = 47
	// [1,0]: 1*4 + 2*5 + 3*7 + 4*8 = 4+10+21+32 = 67
	// [1,1]: 1*5 + 2*6 + 3*8 + 4*9 = 5+12+24+36 = 77
	expected := []float32{37, 47, 67, 77}

	outputData := output.Raw().AsFloat32()
	for i, exp := range expected {
		if outputData[i] != exp {
			t.Errorf("Output[%d]: expected %.1f, got %.1f", i, exp, outputData[i])
		}
	}
}

// TestConv2D_WithBias tests forward pass with bias.
func TestConv2D_WithBias(t *testing.T) {
	backend := cpu.New()

	conv := NewConv2D(1, 2, 2, 2, 1, 0, true, backend)

	// Set weights to ones
	weightData := conv.weight.Tensor().Raw().AsFloat32()
	for i := range weightData {
		weightData[i] = 1.0
	}

	// Set biases to [10, 20]
	biasData := conv.bias.Tensor().Raw().AsFloat32()
	biasData[0], biasData[1] = 10.0, 20.0

	// Input: [1, 1, 2, 2] all ones
	input := tensor.Ones[float32](tensor.Shape{1, 1, 2, 2}, backend)

	// Forward
	output := conv.Forward(input)

	// Without bias: 1+1+1+1 = 4
	// With bias channel 0: 4 + 10 = 14
	// With bias channel 1: 4 + 20 = 24
	outputData := output.Raw().AsFloat32()

	if outputData[0] != 14.0 {
		t.Errorf("Output channel 0: expected 14, got %.1f", outputData[0])
	}
	if outputData[1] != 24.0 {
		t.Errorf("Output channel 1: expected 24, got %.1f", outputData[1])
	}
}

// TestConv2D_ComputeOutputSize tests output size computation.
func TestConv2D_ComputeOutputSize(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		kernelH, kernelW     int
		stride, padding      int
		inputH, inputW       int
		expectedH, expectedW int
	}{
		{5, 5, 1, 0, 28, 28, 24, 24}, // MNIST typical
		{3, 3, 1, 1, 28, 28, 28, 28}, // same padding
		{3, 3, 2, 0, 28, 28, 13, 13}, // stride 2
		{2, 2, 2, 0, 4, 4, 2, 2},     // simple downsample
	}

	for _, tt := range tests {
		conv := NewConv2D(1, 1, tt.kernelH, tt.kernelW, tt.stride, tt.padding, false, backend)
		outSize := conv.ComputeOutputSize(tt.inputH, tt.inputW)

		if outSize[0] != tt.expectedH || outSize[1] != tt.expectedW {
			t.Errorf("ComputeOutputSize(kernel=%dx%d, stride=%d, padding=%d, input=%dx%d): expected [%d,%d], got %v",
				tt.kernelH, tt.kernelW, tt.stride, tt.padding, tt.inputH, tt.inputW,
				tt.expectedH, tt.expectedW, outSize)
		}
	}
}

// TestConv2D_IntegrationWithAutodiff tests Conv2D with autodiff backend.
func TestConv2D_IntegrationWithAutodiff(t *testing.T) {
	cpuBackend := cpu.New()
	backend := autodiff.New(cpuBackend)

	// Start recording
	backend.Tape().StartRecording()

	// Create Conv2D layer
	conv := NewConv2D(1, 2, 3, 3, 1, 0, true, backend)

	// Input: [1, 1, 5, 5]
	input := tensor.Randn[float32](tensor.Shape{1, 1, 5, 5}, backend)

	// Forward
	output := conv.Forward(input)

	// Check output shape: [1, 2, 3, 3]
	expectedShape := tensor.Shape{1, 2, 3, 3}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape: expected %v, got %v", expectedShape, output.Shape())
	}

	// Create dummy loss (sum of outputs)
	lossVal := float32(0.0)
	outputData := output.Raw().AsFloat32()
	for _, v := range outputData {
		lossVal += v
	}

	// Create output gradient (all ones for sum loss)
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, backend.Device())
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward pass
	grads := backend.Tape().Backward(outputGrad, backend)

	// Debug: Print all gradient keys
	t.Logf("Total gradients computed: %d", len(grads))
	t.Logf("Weight pointer: %p", conv.weight.Tensor().Raw())
	t.Logf("Bias pointer: %p", conv.bias.Tensor().Raw())

	// Print all gradient keys
	for k := range grads {
		t.Logf("Gradient key: %p (shape=%v)", k, k.Shape())
	}

	// Check that weight and bias gradients exist
	weightGrad, hasWeightGrad := grads[conv.weight.Tensor().Raw()]
	biasGrad, hasBiasGrad := grads[conv.bias.Tensor().Raw()]

	if !hasWeightGrad {
		t.Error("No gradient for weight!")
	}
	if !hasBiasGrad {
		t.Error("No gradient for bias!")
		// Don't panic, just fail the test
		return
	}

	// Verify gradients are non-zero
	weightGradData := weightGrad.AsFloat32()
	biasGradData := biasGrad.AsFloat32()

	weightNonZero := 0
	for _, g := range weightGradData {
		if g != 0.0 {
			weightNonZero++
		}
	}

	biasNonZero := 0
	for _, g := range biasGradData {
		if g != 0.0 {
			biasNonZero++
		}
	}

	if weightNonZero == 0 {
		t.Error("Weight gradient has all zeros!")
	}
	if biasNonZero == 0 {
		t.Error("Bias gradient has all zeros!")
	}

	t.Logf("SUCCESS: Conv2D integrates correctly with autodiff")
	t.Logf("  Weight gradient: %d/%d non-zero", weightNonZero, len(weightGradData))
	t.Logf("  Bias gradient: %d/%d non-zero", biasNonZero, len(biasGradData))
}
