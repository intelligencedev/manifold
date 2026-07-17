package ops

import (
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestMaxPool2DOp_BackwardGradients tests MaxPool2D backward pass gradients.
func TestMaxPool2DOp_BackwardGradients(t *testing.T) {
	backend := cpu.New()

	// Input: [1, 1, 4, 4] with sequential values
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 4, 4}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := 0; i < 16; i++ {
		inputData[i] = float32(i + 1)
	}

	// Forward with 2x2 kernel, stride=2
	output := backend.MaxPool2D(input, 2, 2)

	// Create operation
	op := NewMaxPool2DOp(input, output, 2, 2)

	// Output gradient (all ones)
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, tensor.CPU)
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward
	grads := op.Backward(outputGrad, backend)

	// Check we got 1 gradient (input only, no learnable parameters)
	if len(grads) != 1 {
		t.Fatalf("Expected 1 gradient, got %d", len(grads))
	}

	inputGrad := grads[0]

	// Verify shape
	if !inputGrad.Shape().Equal(input.Shape()) {
		t.Errorf("inputGrad shape %v != input shape %v", inputGrad.Shape(), input.Shape())
	}

	// Verify gradient routing
	// Input: [[1,2,3,4],     Max positions: [6,8,14,16]
	//         [5,6,7,8],
	//         [9,10,11,12],
	//         [13,14,15,16]]
	//
	// Expected gradient: Only positions 5,7,13,15 (0-indexed) should have gradient 1.0
	inputGradData := inputGrad.AsFloat32()

	expectedNonZero := map[int]float32{
		5:  1.0, // position of 6
		7:  1.0, // position of 8
		13: 1.0, // position of 14
		15: 1.0, // position of 16
	}

	for i, grad := range inputGradData {
		expectedGrad, shouldBeNonZero := expectedNonZero[i]
		if shouldBeNonZero {
			if grad != expectedGrad {
				t.Errorf("inputGrad[%d]: expected %.1f (max position), got %.1f", i, expectedGrad, grad)
			}
		} else {
			if grad != 0.0 {
				t.Errorf("inputGrad[%d]: expected 0.0 (non-max position), got %.1f", i, grad)
			}
		}
	}

	t.Log("SUCCESS: MaxPool2D gradients routed correctly to max positions")
}

// TestMaxPool2DOp_GradientAccumulation tests gradient accumulation for overlapping windows.
func TestMaxPool2DOp_GradientAccumulation(t *testing.T) {
	backend := cpu.New()

	// Input: [1, 1, 5, 5] with all same values
	// This tests gradient accumulation when same position is max in multiple windows
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 5, 5}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := 0; i < 25; i++ {
		inputData[i] = 1.0 // All same value
	}

	// MaxPool with 3x3 kernel, stride=1 (overlapping windows)
	output := backend.MaxPool2D(input, 3, 1)

	// Create operation
	op := NewMaxPool2DOp(input, output, 3, 1)

	// Output gradient (all ones)
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, tensor.CPU)
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward
	grads := op.Backward(outputGrad, backend)
	inputGrad := grads[0]
	inputGradData := inputGrad.AsFloat32()

	// With overlapping windows and same values, gradients should accumulate
	// Total gradient sum should equal number of output positions
	totalGrad := float32(0.0)
	for _, grad := range inputGradData {
		totalGrad += grad
	}

	expectedTotal := float32(len(outputGradData))
	if totalGrad != expectedTotal {
		t.Errorf("Total gradient: expected %.1f, got %.1f", expectedTotal, totalGrad)
	}

	t.Logf("Gradient accumulation correct: total=%.1f", totalGrad)
}

// TestMaxPool2DOp_MultiChannel tests gradients with multiple channels.
func TestMaxPool2DOp_MultiChannel(t *testing.T) {
	backend := cpu.New()

	// Input: [1, 2, 4, 4] (2 channels)
	input, _ := tensor.NewRaw(tensor.Shape{1, 2, 4, 4}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()

	// Channel 0: sequential 1-16
	for i := 0; i < 16; i++ {
		inputData[i] = float32(i + 1)
	}
	// Channel 1: sequential 17-32
	for i := 16; i < 32; i++ {
		inputData[i] = float32(i + 1)
	}

	// MaxPool 2x2, stride=2
	output := backend.MaxPool2D(input, 2, 2)

	// Create operation
	op := NewMaxPool2DOp(input, output, 2, 2)

	// Output gradient (all ones)
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, tensor.CPU)
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward
	grads := op.Backward(outputGrad, backend)
	inputGrad := grads[0]
	inputGradData := inputGrad.AsFloat32()

	// Verify each channel has correct number of non-zero gradients
	channel0NonZero := 0
	channel1NonZero := 0

	for i := 0; i < 16; i++ {
		if inputGradData[i] != 0.0 {
			channel0NonZero++
		}
	}
	for i := 16; i < 32; i++ {
		if inputGradData[i] != 0.0 {
			channel1NonZero++
		}
	}

	// Each channel should have 4 non-zero gradients (one per output position)
	if channel0NonZero != 4 {
		t.Errorf("Channel 0: expected 4 non-zero gradients, got %d", channel0NonZero)
	}
	if channel1NonZero != 4 {
		t.Errorf("Channel 1: expected 4 non-zero gradients, got %d", channel1NonZero)
	}

	t.Log("SUCCESS: Multi-channel gradients correct")
}

// TestMaxPool2DOp_Batch tests gradients with batch processing.
func TestMaxPool2DOp_Batch(t *testing.T) {
	backend := cpu.New()

	// Input: [2, 1, 4, 4] (batch size 2)
	input, _ := tensor.NewRaw(tensor.Shape{2, 1, 4, 4}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()

	// Fill with distinct values per batch
	for i := 0; i < 32; i++ {
		inputData[i] = float32(i + 1)
	}

	// MaxPool 2x2, stride=2
	output := backend.MaxPool2D(input, 2, 2)

	// Create operation
	op := NewMaxPool2DOp(input, output, 2, 2)

	// Output gradient (all ones)
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float32, tensor.CPU)
	outputGradData := outputGrad.AsFloat32()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward
	grads := op.Backward(outputGrad, backend)
	inputGrad := grads[0]
	inputGradData := inputGrad.AsFloat32()

	// Verify each batch has correct number of non-zero gradients
	batch0NonZero := 0
	batch1NonZero := 0

	for i := 0; i < 16; i++ {
		if inputGradData[i] != 0.0 {
			batch0NonZero++
		}
	}
	for i := 16; i < 32; i++ {
		if inputGradData[i] != 0.0 {
			batch1NonZero++
		}
	}

	// Each batch should have 4 non-zero gradients
	if batch0NonZero != 4 {
		t.Errorf("Batch 0: expected 4 non-zero gradients, got %d", batch0NonZero)
	}
	if batch1NonZero != 4 {
		t.Errorf("Batch 1: expected 4 non-zero gradients, got %d", batch1NonZero)
	}

	t.Log("SUCCESS: Batch gradients correct")
}

// TestMaxPool2DOp_Float64 tests float64 support.
func TestMaxPool2DOp_Float64(t *testing.T) {
	backend := cpu.New()

	// Input: [1, 1, 4, 4] float64
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 4, 4}, tensor.Float64, tensor.CPU)
	inputData := input.AsFloat64()
	for i := 0; i < 16; i++ {
		inputData[i] = float64(i + 1)
	}

	// Forward
	output := backend.MaxPool2D(input, 2, 2)

	// Create operation
	op := NewMaxPool2DOp(input, output, 2, 2)

	// Output gradient
	outputGrad, _ := tensor.NewRaw(output.Shape(), tensor.Float64, tensor.CPU)
	outputGradData := outputGrad.AsFloat64()
	for i := range outputGradData {
		outputGradData[i] = 1.0
	}

	// Backward
	grads := op.Backward(outputGrad, backend)
	inputGrad := grads[0]
	inputGradData := inputGrad.AsFloat64()

	// Verify non-zero gradients at max positions
	nonZeroCount := 0
	for _, grad := range inputGradData {
		if grad != 0.0 {
			nonZeroCount++
		}
	}

	if nonZeroCount != 4 {
		t.Errorf("Expected 4 non-zero gradients, got %d", nonZeroCount)
	}

	t.Log("SUCCESS: Float64 gradients correct")
}

func BenchmarkMaxPool2D_Backward_Batch(b *testing.B) {
	backend := cpu.New()

	input := tensor.Randn[float32](tensor.Shape{64, 1, 28, 28}, backend).Raw()
	grad := tensor.Randn[float32](tensor.Shape{64, 1, 14, 14}, backend).Raw()

	output := backend.MaxPool2D(input, 2, 2)
	op := NewMaxPool2DOp(input, output, 2, 2)

	b.ResetTimer()
	for b.Loop() {
		backend.MaxPool2DBackward(input, grad, op.maxIndices, 2, 2)
	}
}

func BenchmarkMaxPool2D_Backward_MultiChannel(b *testing.B) {
	backend := cpu.New()

	input := tensor.Randn[float32](tensor.Shape{1, 16, 14, 14}, backend).Raw()
	grad := tensor.Randn[float32](tensor.Shape{1, 16, 12, 12}, backend).Raw()

	output := backend.MaxPool2D(input, 3, 1)
	op := NewMaxPool2DOp(input, output, 3, 1)

	b.ResetTimer()
	for b.Loop() {
		backend.MaxPool2DBackward(input, grad, op.maxIndices, 3, 1)
	}
}

func BenchmarkMaxPool2D_Backward_Deep(b *testing.B) {
	backend := cpu.New()

	input := tensor.Randn[float32](tensor.Shape{8, 64, 14, 14}, backend).Raw()
	grad := tensor.Randn[float32](tensor.Shape{8, 64, 12, 12}, backend).Raw()

	output := backend.MaxPool2D(input, 3, 1)
	op := NewMaxPool2DOp(input, output, 3, 1)

	b.ResetTimer()
	for b.Loop() {
		backend.MaxPool2DBackward(input, grad, op.maxIndices, 3, 1)
	}
}
