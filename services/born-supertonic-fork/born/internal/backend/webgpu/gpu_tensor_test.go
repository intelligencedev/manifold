//go:build windows || linux

package webgpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestGPUTensorFromRawTensor tests uploading CPU tensor to GPU.
func TestGPUTensorFromRawTensor(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create CPU tensor
	cpuTensor, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create CPU tensor: %v", err)
	}

	// Fill with test data
	data := cpuTensor.AsFloat32()
	for i := range data {
		data[i] = float32(i + 1)
	}

	// Upload to GPU
	gpuTensor := backend.FromRawTensor(cpuTensor)
	defer gpuTensor.Release()

	// Verify metadata
	if !gpuTensor.Shape().Equal(cpuTensor.Shape()) {
		t.Errorf("Shape mismatch: got %v, want %v", gpuTensor.Shape(), cpuTensor.Shape())
	}

	if gpuTensor.DType() != cpuTensor.DType() {
		t.Errorf("DType mismatch: got %v, want %v", gpuTensor.DType(), cpuTensor.DType())
	}

	if gpuTensor.NumElements() != 6 {
		t.Errorf("NumElements: got %d, want 6", gpuTensor.NumElements())
	}

	if !gpuTensor.computed {
		t.Error("GPUTensor should be marked as computed after FromRawTensor")
	}
}

// TestGPUTensorToCPU tests transferring GPU tensor back to CPU.
func TestGPUTensorToCPU(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create CPU tensor with known data
	cpuTensor, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create CPU tensor: %v", err)
	}

	data := cpuTensor.AsFloat32()
	expected := []float32{1.0, 2.0, 3.0, 4.0}
	copy(data, expected)

	// Upload to GPU
	gpuTensor := backend.FromRawTensor(cpuTensor)
	defer gpuTensor.Release()

	// Transfer back to CPU
	result := gpuTensor.ToCPU()

	// Verify shape and dtype
	if !result.Shape().Equal(tensor.Shape{2, 2}) {
		t.Errorf("Shape mismatch: got %v, want [2 2]", result.Shape())
	}

	if result.DType() != tensor.Float32 {
		t.Errorf("DType mismatch: got %v, want Float32", result.DType())
	}

	// Verify data
	resultData := result.AsFloat32()
	if len(resultData) != len(expected) {
		t.Fatalf("Data length mismatch: got %d, want %d", len(resultData), len(expected))
	}

	for i, val := range resultData {
		if val != expected[i] {
			t.Errorf("Data[%d]: got %f, want %f", i, val, expected[i])
		}
	}
}

// TestGPUTensorItem tests extracting scalar value.
func TestGPUTensorItem(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	tests := []struct {
		name     string
		dtype    tensor.DataType
		setValue interface{}
		want     float32
	}{
		{"Float32", tensor.Float32, float32(42.5), 42.5},
		{"Float64", tensor.Float64, float64(3.14), 3.14},
		{"Int32", tensor.Int32, int32(100), 100.0},
		{"Int64", tensor.Int64, int64(256), 256.0},
		{"Uint8", tensor.Uint8, uint8(8), 8.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create scalar CPU tensor
			cpuTensor, err := tensor.NewRaw(tensor.Shape{1}, tt.dtype, tensor.CPU)
			if err != nil {
				t.Fatalf("Failed to create CPU tensor: %v", err)
			}

			// Set value based on dtype
			switch tt.dtype {
			case tensor.Float32:
				cpuTensor.AsFloat32()[0] = tt.setValue.(float32)
			case tensor.Float64:
				cpuTensor.AsFloat64()[0] = tt.setValue.(float64)
			case tensor.Int32:
				cpuTensor.AsInt32()[0] = tt.setValue.(int32)
			case tensor.Int64:
				cpuTensor.AsInt64()[0] = tt.setValue.(int64)
			case tensor.Uint8:
				cpuTensor.AsUint8()[0] = tt.setValue.(uint8)
			}

			// Upload to GPU
			gpuTensor := backend.FromRawTensor(cpuTensor)
			defer gpuTensor.Release()

			// Extract scalar
			got := gpuTensor.Item()

			if got != tt.want {
				t.Errorf("Item(): got %f, want %f", got, tt.want)
			}
		})
	}
}

// TestGPUTensorItemPanic tests that Item() panics on non-scalar tensors.
func TestGPUTensorItemPanic(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	// Create non-scalar tensor
	cpuTensor, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("Failed to create CPU tensor: %v", err)
	}

	gpuTensor := backend.FromRawTensor(cpuTensor)
	defer gpuTensor.Release()

	// Expect panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Item() should panic on non-scalar tensor")
		}
	}()

	gpuTensor.Item()
}

// TestGPUTensorShape tests Shape() method.
func TestGPUTensorShape(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	tests := []struct {
		name  string
		shape tensor.Shape
	}{
		{"Scalar", tensor.Shape{1}},
		{"Vector", tensor.Shape{10}},
		{"Matrix", tensor.Shape{3, 4}},
		{"3D", tensor.Shape{2, 3, 4}},
		{"4D", tensor.Shape{2, 3, 4, 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpuTensor, err := tensor.NewRaw(tt.shape, tensor.Float32, tensor.CPU)
			if err != nil {
				t.Fatalf("Failed to create CPU tensor: %v", err)
			}

			gpuTensor := backend.FromRawTensor(cpuTensor)
			defer gpuTensor.Release()

			if !gpuTensor.Shape().Equal(tt.shape) {
				t.Errorf("Shape(): got %v, want %v", gpuTensor.Shape(), tt.shape)
			}
		})
	}
}

// TestZerosGPU tests creating zero-filled GPU tensors.
//
//nolint:gocognit // Test function with multiple validation checks
func TestZerosGPU(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	tests := []struct {
		name  string
		shape tensor.Shape
		dtype tensor.DataType
	}{
		{"Float32Vector", tensor.Shape{10}, tensor.Float32},
		{"Float32Matrix", tensor.Shape{3, 4}, tensor.Float32},
		{"Int32Vector", tensor.Shape{5}, tensor.Int32},
		{"Float64Matrix", tensor.Shape{2, 3}, tensor.Float64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpuTensor := backend.ZerosGPU(tt.shape, tt.dtype)
			defer gpuTensor.Release()

			// Verify metadata
			if !gpuTensor.Shape().Equal(tt.shape) {
				t.Errorf("Shape: got %v, want %v", gpuTensor.Shape(), tt.shape)
			}

			if gpuTensor.DType() != tt.dtype {
				t.Errorf("DType: got %v, want %v", gpuTensor.DType(), tt.dtype)
			}

			// Transfer to CPU and verify zeros
			cpuTensor := gpuTensor.ToCPU()

			switch tt.dtype {
			case tensor.Float32:
				data := cpuTensor.AsFloat32()
				for i, val := range data {
					if val != 0.0 {
						t.Errorf("Data[%d]: got %f, want 0.0", i, val)
					}
				}
			case tensor.Float64:
				data := cpuTensor.AsFloat64()
				for i, val := range data {
					if val != 0.0 {
						t.Errorf("Data[%d]: got %f, want 0.0", i, val)
					}
				}
			case tensor.Int32:
				data := cpuTensor.AsInt32()
				for i, val := range data {
					if val != 0 {
						t.Errorf("Data[%d]: got %d, want 0", i, val)
					}
				}
			}
		})
	}
}

// TestOnesGPU tests creating GPU tensors filled with ones.
//
//nolint:gocognit // Test function with multiple validation checks
func TestOnesGPU(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	tests := []struct {
		name  string
		shape tensor.Shape
		dtype tensor.DataType
	}{
		{"Float32Vector", tensor.Shape{5}, tensor.Float32},
		{"Float32Matrix", tensor.Shape{2, 3}, tensor.Float32},
		{"Int32Vector", tensor.Shape{4}, tensor.Int32},
		{"Float64Matrix", tensor.Shape{3, 2}, tensor.Float64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpuTensor := backend.OnesGPU(tt.shape, tt.dtype)
			defer gpuTensor.Release()

			// Verify metadata
			if !gpuTensor.Shape().Equal(tt.shape) {
				t.Errorf("Shape: got %v, want %v", gpuTensor.Shape(), tt.shape)
			}

			// Transfer to CPU and verify ones
			cpuTensor := gpuTensor.ToCPU()

			switch tt.dtype {
			case tensor.Float32:
				data := cpuTensor.AsFloat32()
				for i, val := range data {
					if val != 1.0 {
						t.Errorf("Data[%d]: got %f, want 1.0", i, val)
					}
				}
			case tensor.Float64:
				data := cpuTensor.AsFloat64()
				for i, val := range data {
					if val != 1.0 {
						t.Errorf("Data[%d]: got %f, want 1.0", i, val)
					}
				}
			case tensor.Int32:
				data := cpuTensor.AsInt32()
				for i, val := range data {
					if val != 1 {
						t.Errorf("Data[%d]: got %d, want 1", i, val)
					}
				}
			}
		})
	}
}

// TestRandGPU tests creating random GPU tensors.
//
//nolint:gocognit // Test function with multiple validation checks
func TestRandGPU(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	tests := []struct {
		name  string
		shape tensor.Shape
		dtype tensor.DataType
	}{
		{"Float32Vector", tensor.Shape{100}, tensor.Float32},
		{"Float32Matrix", tensor.Shape{10, 10}, tensor.Float32},
		{"Int32Vector", tensor.Shape{50}, tensor.Int32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpuTensor := backend.RandGPU(tt.shape, tt.dtype)
			defer gpuTensor.Release()

			// Verify metadata
			if !gpuTensor.Shape().Equal(tt.shape) {
				t.Errorf("Shape: got %v, want %v", gpuTensor.Shape(), tt.shape)
			}

			// Transfer to CPU and verify random data
			cpuTensor := gpuTensor.ToCPU()

			// Check that not all values are the same (very unlikely for random data)
			switch tt.dtype {
			case tensor.Float32:
				data := cpuTensor.AsFloat32()
				if len(data) > 1 {
					allSame := true
					firstVal := data[0]
					for _, val := range data[1:] {
						if val != firstVal {
							allSame = false
							break
						}
					}
					if allSame {
						t.Error("Random data should not have all identical values")
					}

					// Check range [0, 1) for float
					for i, val := range data {
						if val < 0.0 || val >= 1.0 {
							t.Errorf("Data[%d]: got %f, want range [0, 1)", i, val)
						}
					}
				}
			case tensor.Int32:
				data := cpuTensor.AsInt32()
				if len(data) > 1 {
					allSame := true
					firstVal := data[0]
					for _, val := range data[1:] {
						if val != firstVal {
							allSame = false
							break
						}
					}
					if allSame {
						t.Error("Random data should not have all identical values")
					}
				}
			}
		})
	}
}

// TestGPUTensorEval tests Eval() method.
func TestGPUTensorEval(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	gpuTensor := backend.ZerosGPU(tensor.Shape{2, 2}, tensor.Float32)
	defer gpuTensor.Release()

	// Initially computed should be true for ZerosGPU
	if !gpuTensor.computed {
		t.Error("ZerosGPU should create computed tensor")
	}

	// Mark as not computed for testing
	gpuTensor.computed = false

	// Call Eval()
	result := gpuTensor.Eval()

	if result != gpuTensor {
		t.Error("Eval() should return same tensor")
	}

	if !result.computed {
		t.Error("Eval() should mark tensor as computed")
	}
}

// TestGPUTensorByteSize tests ByteSize() method.
func TestGPUTensorByteSize(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Release()

	tests := []struct {
		name     string
		shape    tensor.Shape
		dtype    tensor.DataType
		wantSize uint64
	}{
		{"Float32_2x3", tensor.Shape{2, 3}, tensor.Float32, 24}, // 6 * 4 bytes
		{"Float64_2x2", tensor.Shape{2, 2}, tensor.Float64, 32}, // 4 * 8 bytes
		{"Int32_10", tensor.Shape{10}, tensor.Int32, 40},        // 10 * 4 bytes
		{"Uint8_100", tensor.Shape{100}, tensor.Uint8, 100},     // 100 * 1 byte
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpuTensor := backend.ZerosGPU(tt.shape, tt.dtype)
			defer gpuTensor.Release()

			got := gpuTensor.ByteSize()
			if got != tt.wantSize {
				t.Errorf("ByteSize(): got %d, want %d", got, tt.wantSize)
			}
		})
	}
}
