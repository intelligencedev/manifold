//go:build windows || linux

package webgpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// Test 3D transpose.
func TestTranspose3D(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create 3D tensor [2, 3, 4]
	data := make([]float32, 24)
	for i := 0; i < 24; i++ {
		data[i] = float32(i)
	}

	input, err := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, tensor.WebGPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(input.AsFloat32(), data)

	// Transpose [2, 3, 4] -> [2, 4, 3] (axes = [0, 2, 1])
	result := backend.Transpose(input, 0, 2, 1)

	// Verify shape
	expectedShape := tensor.Shape{2, 4, 3}
	if !result.Shape().Equal(expectedShape) {
		t.Errorf("Expected shape %v, got %v", expectedShape, result.Shape())
	}

	// Verify data
	resultData := result.AsFloat32()

	// Original:  [batch=0, row=0, col=[0,1,2,3]], [batch=0, row=1, col=[4,5,6,7]], ...
	// After:     [batch=0, col=0, row=[0,4,8]], [batch=0, col=1, row=[1,5,9]], ...

	// Check a few key values
	// Original[0,0,0] = 0 -> Result[0,0,0] = 0
	if resultData[0] != 0 {
		t.Errorf("Expected resultData[0] = 0, got %f", resultData[0])
	}
	// Original[0,0,1] = 1 -> Result[0,1,0] = 1
	if resultData[3] != 1 {
		t.Errorf("Expected resultData[3] = 1, got %f", resultData[3])
	}
	// Original[0,1,0] = 4 -> Result[0,0,1] = 4
	if resultData[1] != 4 {
		t.Errorf("Expected resultData[1] = 4, got %f", resultData[1])
	}
}

// Test 4D transpose for attention mechanism.
func TestTranspose4D(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create 4D tensor [2, 8, 16, 64] (batch, heads, seq_len, dim)
	shape := tensor.Shape{2, 8, 16, 64}
	numElements := shape.NumElements()
	data := make([]float32, numElements)
	for i := 0; i < numElements; i++ {
		data[i] = float32(i)
	}

	input, err := tensor.NewRaw(shape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(input.AsFloat32(), data)

	// Transpose [2, 8, 16, 64] -> [2, 16, 8, 64] (axes = [0, 2, 1, 3])
	// This is a common operation in multi-head attention
	result := backend.Transpose(input, 0, 2, 1, 3)

	// Verify shape
	expectedShape := tensor.Shape{2, 16, 8, 64}
	if !result.Shape().Equal(expectedShape) {
		t.Errorf("Expected shape %v, got %v", expectedShape, result.Shape())
	}

	// Verify dimensions
	if len(result.Shape()) != 4 {
		t.Errorf("Expected 4D tensor, got %dD", len(result.Shape()))
	}
}

// Test 4D transpose with int32.
func TestTranspose4DInt32(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create 4D int32 tensor [2, 3, 4, 5]
	shape := tensor.Shape{2, 3, 4, 5}
	numElements := shape.NumElements()
	data := make([]int32, numElements)
	for i := 0; i < numElements; i++ {
		data[i] = int32(i)
	}

	input, err := tensor.NewRaw(shape, tensor.Int32, tensor.WebGPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(input.AsInt32(), data)

	// Transpose [2, 3, 4, 5] -> [5, 4, 3, 2] (full reverse)
	result := backend.Transpose(input, 3, 2, 1, 0)

	// Verify shape
	expectedShape := tensor.Shape{5, 4, 3, 2}
	if !result.Shape().Equal(expectedShape) {
		t.Errorf("Expected shape %v, got %v", expectedShape, result.Shape())
	}
}

// Test Expand broadcasting.
func TestExpandBroadcast(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create tensor [1, 1, 64]
	data := make([]float32, 64)
	for i := 0; i < 64; i++ {
		data[i] = float32(i)
	}

	input, err := tensor.NewRaw(tensor.Shape{1, 1, 64}, tensor.Float32, tensor.WebGPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(input.AsFloat32(), data)

	// Expand [1, 1, 64] -> [2, 16, 64]
	newShape := tensor.Shape{2, 16, 64}
	result := backend.Expand(input, newShape)

	// Verify shape
	if !result.Shape().Equal(newShape) {
		t.Errorf("Expected shape %v, got %v", newShape, result.Shape())
	}

	// Verify broadcasting: all [b, i, :] should have same values as input[0, 0, :]
	resultData := result.AsFloat32()
	for b := 0; b < 2; b++ {
		for i := 0; i < 16; i++ {
			for j := 0; j < 64; j++ {
				idx := b*16*64 + i*64 + j
				expected := data[j]
				if resultData[idx] != expected {
					t.Errorf("At [%d,%d,%d]: expected %f, got %f", b, i, j, expected, resultData[idx])
				}
			}
		}
	}
}

// Test Expand with partial broadcasting.
func TestExpandPartialBroadcast(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create tensor [3, 1, 4]
	data := make([]float32, 12)
	for i := 0; i < 12; i++ {
		data[i] = float32(i)
	}

	input, err := tensor.NewRaw(tensor.Shape{3, 1, 4}, tensor.Float32, tensor.WebGPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(input.AsFloat32(), data)

	// Expand [3, 1, 4] -> [3, 5, 4]
	newShape := tensor.Shape{3, 5, 4}
	result := backend.Expand(input, newShape)

	// Verify shape
	if !result.Shape().Equal(newShape) {
		t.Errorf("Expected shape %v, got %v", newShape, result.Shape())
	}

	// Verify broadcasting: result[b, i, j] should equal input[b, 0, j]
	resultData := result.AsFloat32()
	for b := 0; b < 3; b++ {
		for i := 0; i < 5; i++ {
			for j := 0; j < 4; j++ {
				resultIdx := b*5*4 + i*4 + j
				inputIdx := b*4 + j
				expected := data[inputIdx]
				if resultData[resultIdx] != expected {
					t.Errorf("At [%d,%d,%d]: expected %f, got %f", b, i, j, expected, resultData[resultIdx])
				}
			}
		}
	}
}

// Test Expand with int32.
func TestExpandInt32(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create tensor [1, 4]
	data := []int32{10, 20, 30, 40}

	input, err := tensor.NewRaw(tensor.Shape{1, 4}, tensor.Int32, tensor.WebGPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(input.AsInt32(), data)

	// Expand [1, 4] -> [3, 4]
	newShape := tensor.Shape{3, 4}
	result := backend.Expand(input, newShape)

	// Verify shape
	if !result.Shape().Equal(newShape) {
		t.Errorf("Expected shape %v, got %v", newShape, result.Shape())
	}

	// Verify broadcasting
	resultData := result.AsInt32()
	for i := 0; i < 3; i++ {
		for j := 0; j < 4; j++ {
			idx := i*4 + j
			expected := data[j]
			if resultData[idx] != expected {
				t.Errorf("At [%d,%d]: expected %d, got %d", i, j, expected, resultData[idx])
			}
		}
	}
}

// Test 5D transpose.
func TestTranspose5D(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create 5D tensor [2, 3, 4, 5, 6]
	shape := tensor.Shape{2, 3, 4, 5, 6}
	numElements := shape.NumElements()
	data := make([]float32, numElements)
	for i := 0; i < numElements; i++ {
		data[i] = float32(i % 100) // Keep values small for easier debugging
	}

	input, err := tensor.NewRaw(shape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(input.AsFloat32(), data)

	// Transpose [2, 3, 4, 5, 6] -> [6, 5, 4, 3, 2]
	result := backend.Transpose(input, 4, 3, 2, 1, 0)

	// Verify shape
	expectedShape := tensor.Shape{6, 5, 4, 3, 2}
	if !result.Shape().Equal(expectedShape) {
		t.Errorf("Expected shape %v, got %v", expectedShape, result.Shape())
	}
}

// Test 6D expand (max supported).
func TestExpand6D(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create tensor [1, 1, 1, 1, 1, 8]
	data := make([]float32, 8)
	for i := 0; i < 8; i++ {
		data[i] = float32(i)
	}

	input, err := tensor.NewRaw(tensor.Shape{1, 1, 1, 1, 1, 8}, tensor.Float32, tensor.WebGPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(input.AsFloat32(), data)

	// Expand [1, 1, 1, 1, 1, 8] -> [2, 2, 2, 2, 2, 8]
	newShape := tensor.Shape{2, 2, 2, 2, 2, 8}
	result := backend.Expand(input, newShape)

	// Verify shape
	if !result.Shape().Equal(newShape) {
		t.Errorf("Expected shape %v, got %v", newShape, result.Shape())
	}

	// Verify a few values
	resultData := result.AsFloat32()
	// All positions [..., :] should have data[:]
	if resultData[0] != 0 {
		t.Errorf("Expected resultData[0] = 0, got %f", resultData[0])
	}
	if resultData[7] != 7 {
		t.Errorf("Expected resultData[7] = 7, got %f", resultData[7])
	}
	// Last element
	lastIdx := newShape.NumElements() - 1
	if resultData[lastIdx] != 7 {
		t.Errorf("Expected resultData[%d] = 7, got %f", lastIdx, resultData[lastIdx])
	}
}

// Test transpose no-op.
func TestTransposeNoOp(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create 3D tensor
	data := make([]float32, 24)
	for i := 0; i < 24; i++ {
		data[i] = float32(i)
	}

	input, err := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, tensor.WebGPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(input.AsFloat32(), data)

	// Transpose with identity axes [0, 1, 2]
	result := backend.Transpose(input, 0, 1, 2)

	// Should return the same tensor (pointer equality)
	if result != input {
		t.Error("Expected no-op transpose to return same tensor")
	}
}
