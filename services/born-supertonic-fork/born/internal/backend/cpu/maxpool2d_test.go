package cpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestMaxPool2D_BasicForward tests basic max pooling correctness.
func TestMaxPool2D_BasicForward(t *testing.T) {
	backend := New()

	// Input: [1, 1, 4, 4] with sequential values 1-16
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 4, 4}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := 0; i < 16; i++ {
		inputData[i] = float32(i + 1)
	}

	// MaxPool2D with 2x2 kernel, stride=2
	output := backend.MaxPool2D(input, 2, 2)

	// Expected output: [1, 1, 2, 2]
	expectedShape := tensor.Shape{1, 1, 2, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape: expected %v, got %v", expectedShape, output.Shape())
	}

	// Expected values (max in each 2x2 window):
	// [[1,2,3,4],      -> [[6,8],
	//  [5,6,7,8],         [14,16]]
	//  [9,10,11,12],
	//  [13,14,15,16]]
	expected := []float32{6, 8, 14, 16}
	outputData := output.AsFloat32()

	for i, exp := range expected {
		if outputData[i] != exp {
			t.Errorf("Output[%d]: expected %.1f, got %.1f", i, exp, outputData[i])
		}
	}
}

// TestMaxPool2D_WithStride tests max pooling with different stride.
func TestMaxPool2D_WithStride(t *testing.T) {
	backend := New()

	// Input: [1, 1, 5, 5]
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 5, 5}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := 0; i < 25; i++ {
		inputData[i] = float32(i + 1)
	}

	// MaxPool2D with 3x3 kernel, stride=1
	output := backend.MaxPool2D(input, 3, 1)

	// Expected output: [1, 1, 3, 3]
	// out_h = (5 - 3) / 1 + 1 = 3
	expectedShape := tensor.Shape{1, 1, 3, 3}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape: expected %v, got %v", expectedShape, output.Shape())
	}

	// Verify first output (max of top-left 3x3 window)
	// [[1,2,3],
	//  [6,7,8],
	//  [11,12,13]] -> max = 13
	outputData := output.AsFloat32()
	if outputData[0] != 13 {
		t.Errorf("First output: expected 13, got %.1f", outputData[0])
	}
}

// TestMaxPool2D_MultiChannel tests multi-channel max pooling.
func TestMaxPool2D_MultiChannel(t *testing.T) {
	backend := New()

	// Input: [1, 3, 4, 4] (3 channels)
	input, _ := tensor.NewRaw(tensor.Shape{1, 3, 4, 4}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()

	// Channel 0: all ones
	for i := 0; i < 16; i++ {
		inputData[i] = 1.0
	}
	// Channel 1: all twos
	for i := 16; i < 32; i++ {
		inputData[i] = 2.0
	}
	// Channel 2: all threes
	for i := 32; i < 48; i++ {
		inputData[i] = 3.0
	}

	// MaxPool2D 2x2, stride=2
	output := backend.MaxPool2D(input, 2, 2)

	// Expected output: [1, 3, 2, 2]
	expectedShape := tensor.Shape{1, 3, 2, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape: expected %v, got %v", expectedShape, output.Shape())
	}

	outputData := output.AsFloat32()

	// Verify each channel maintains its values
	for c := 0; c < 3; c++ {
		expectedVal := float32(c + 1)
		for i := 0; i < 4; i++ {
			idx := c*4 + i
			if outputData[idx] != expectedVal {
				t.Errorf("Channel %d, output[%d]: expected %.1f, got %.1f",
					c, i, expectedVal, outputData[idx])
			}
		}
	}
}

// TestMaxPool2D_Batch tests batch processing.
func TestMaxPool2D_Batch(t *testing.T) {
	backend := New()

	// Input: [2, 1, 4, 4] (batch size 2)
	input, _ := tensor.NewRaw(tensor.Shape{2, 1, 4, 4}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()

	// Batch 0: values 1-16
	for i := 0; i < 16; i++ {
		inputData[i] = float32(i + 1)
	}
	// Batch 1: values 17-32
	for i := 16; i < 32; i++ {
		inputData[i] = float32(i + 1)
	}

	// MaxPool2D 2x2, stride=2
	output := backend.MaxPool2D(input, 2, 2)

	// Expected output: [2, 1, 2, 2]
	expectedShape := tensor.Shape{2, 1, 2, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Errorf("Output shape: expected %v, got %v", expectedShape, output.Shape())
	}

	outputData := output.AsFloat32()

	// Batch 0: [6, 8, 14, 16]
	expectedBatch0 := []float32{6, 8, 14, 16}
	for i, exp := range expectedBatch0 {
		if outputData[i] != exp {
			t.Errorf("Batch 0, output[%d]: expected %.1f, got %.1f", i, exp, outputData[i])
		}
	}

	// Batch 1: [22, 24, 30, 32]
	expectedBatch1 := []float32{22, 24, 30, 32}
	for i, exp := range expectedBatch1 {
		if outputData[4+i] != exp {
			t.Errorf("Batch 1, output[%d]: expected %.1f, got %.1f", i, exp, outputData[4+i])
		}
	}
}

// TestMaxPool2D_MatchesMockBackend verifies CPU matches naive implementation.
func TestMaxPool2D_MatchesMockBackend(t *testing.T) {
	cpuBackend := New()
	mockBackend := tensor.NewMockBackend()

	// Create test input [1, 2, 6, 6]
	input, _ := tensor.NewRaw(tensor.Shape{1, 2, 6, 6}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i%10 + 1)
	}

	// Test with 3x3 kernel, stride=2
	cpuOutput := cpuBackend.MaxPool2D(input, 3, 2)
	mockOutput := mockBackend.MaxPool2D(input, 3, 2)

	// Verify shapes match
	if !cpuOutput.Shape().Equal(mockOutput.Shape()) {
		t.Fatalf("Shape mismatch: CPU=%v, Mock=%v", cpuOutput.Shape(), mockOutput.Shape())
	}

	// Verify values match
	cpuData := cpuOutput.AsFloat32()
	mockData := mockOutput.AsFloat32()

	for i := range cpuData {
		if cpuData[i] != mockData[i] {
			t.Errorf("Output[%d]: CPU=%.6f, Mock=%.6f", i, cpuData[i], mockData[i])
		}
	}
}

// TestMaxPool2D_Float64 tests float64 support.
func TestMaxPool2D_Float64(t *testing.T) {
	backend := New()

	// Input: [1, 1, 4, 4] float64
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 4, 4}, tensor.Float64, tensor.CPU)
	inputData := input.AsFloat64()
	for i := 0; i < 16; i++ {
		inputData[i] = float64(i + 1)
	}

	// MaxPool2D 2x2, stride=2
	output := backend.MaxPool2D(input, 2, 2)

	// Expected: [6, 8, 14, 16]
	expected := []float64{6, 8, 14, 16}
	outputData := output.AsFloat64()

	for i, exp := range expected {
		if outputData[i] != exp {
			t.Errorf("Output[%d]: expected %.1f, got %.1f", i, exp, outputData[i])
		}
	}
}

func BenchmarkMaxPool2D_Forward(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{1, 1, 28, 28}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.MaxPool2D(input, 2, 2)
	}
}

func BenchmarkMaxPool2D_Forward_Batch(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{64, 1, 28, 28}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.MaxPool2D(input, 2, 2)
	}
}

func BenchmarkMaxPool2D_Forward_MultiChannel(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{1, 16, 14, 14}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.MaxPool2D(input, 3, 1)
	}
}

func BenchmarkMaxPool2D_Forward_Stride2(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{1, 8, 32, 32}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.MaxPool2D(input, 2, 2)
	}
}

func BenchmarkMaxPool2D_Forward_Deep(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{8, 64, 14, 14}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.MaxPool2D(input, 3, 1)
	}
}
