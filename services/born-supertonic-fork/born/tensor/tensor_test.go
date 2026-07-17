// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package tensor_test

import (
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/tensor"
)

// TestBackendInterface verifies that cpu.CPUBackend implements tensor.Backend.
func TestBackendInterface(_ *testing.T) {
	var _ tensor.Backend = (*cpu.CPUBackend)(nil)
}

// TestRawTensorAPI verifies RawTensor type alias exposes expected API.
func TestRawTensorAPI(t *testing.T) {
	raw, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw failed: %v", err)
	}

	// Test Shape() method.
	shape := raw.Shape()
	if !shape.Equal(tensor.Shape{2, 3}) {
		t.Errorf("Shape() = %v, want [2 3]", shape)
	}

	// Test DType() method.
	dtype := raw.DType()
	if dtype != tensor.Float32 {
		t.Errorf("DType() = %v, want Float32", dtype)
	}

	// Test Device() method.
	device := raw.Device()
	if device != tensor.CPU {
		t.Errorf("Device() = %v, want CPU", device)
	}

	// Test NumElements() method.
	n := raw.NumElements()
	if n != 6 {
		t.Errorf("NumElements() = %d, want 6", n)
	}

	// Test ByteSize() method.
	byteSize := raw.ByteSize()
	expected := 6 * 4 // 6 elements * 4 bytes (float32)
	if byteSize != expected {
		t.Errorf("ByteSize() = %d, want %d", byteSize, expected)
	}

	// Test Clone() method.
	clone := raw.Clone()
	if clone == nil {
		t.Error("Clone() returned nil")
	}

	// Test IsUnique() before and after clone.
	if raw.IsUnique() {
		t.Error("IsUnique() = true after Clone(), want false (refcount > 1)")
	}

	// Release clone to restore refcount.
	clone.Release()

	if !raw.IsUnique() {
		t.Error("IsUnique() = false after clone.Release(), want true (refcount == 1)")
	}

	// Test IsLazy() - should be false for CPU tensors.
	if raw.IsLazy() {
		t.Error("IsLazy() = true for CPU tensor, want false")
	}

	// Test Data() method.
	data := raw.Data()
	if len(data) != byteSize {
		t.Errorf("Data() length = %d, want %d", len(data), byteSize)
	}

	// Test AsFloat32() method.
	f32 := raw.AsFloat32()
	if len(f32) != 6 {
		t.Errorf("AsFloat32() length = %d, want 6", len(f32))
	}

	// Test ForceNonUnique() method.
	cleanup := raw.ForceNonUnique()
	if raw.IsUnique() {
		t.Error("IsUnique() = true after ForceNonUnique(), want false")
	}
	cleanup()
	if !raw.IsUnique() {
		t.Error("IsUnique() = false after cleanup(), want true")
	}
}

// TestTensorCreationFunctions verifies high-level tensor creation API.
func TestTensorCreationFunctions(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name string
		fn   func() interface{}
	}{
		{
			name: "Zeros",
			fn: func() interface{} {
				return tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
			},
		},
		{
			name: "Ones",
			fn: func() interface{} {
				return tensor.Ones[float32](tensor.Shape{2, 3}, backend)
			},
		},
		{
			name: "Full",
			fn: func() interface{} {
				return tensor.Full[float32](tensor.Shape{2, 3}, 3.14, backend)
			},
		},
		{
			name: "Randn",
			fn: func() interface{} {
				return tensor.Randn[float32](tensor.Shape{2, 3}, backend)
			},
		},
		{
			name: "Rand",
			fn: func() interface{} {
				return tensor.Rand[float32](tensor.Shape{2, 3}, backend)
			},
		},
		{
			name: "Arange",
			fn: func() interface{} {
				return tensor.Arange[float32](0, 10, backend)
			},
		},
		{
			name: "Eye",
			fn: func() interface{} {
				return tensor.Eye[float32](3, backend)
			},
		},
		{
			name: "FromSlice",
			fn: func() interface{} {
				data := []float32{1, 2, 3, 4, 5, 6}
				t, err := tensor.FromSlice(data, tensor.Shape{2, 3}, backend)
				if err != nil {
					return err
				}
				return t
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn()
			if result == nil {
				t.Errorf("%s() returned nil", tt.name)
			}
			// Check if result is error.
			if err, ok := result.(error); ok {
				t.Errorf("%s() returned error: %v", tt.name, err)
			}
		})
	}
}

// TestDeviceConstants verifies all device constants are accessible.
func TestDeviceConstants(t *testing.T) {
	devices := []struct {
		name   string
		device tensor.Device
	}{
		{"CPU", tensor.CPU},
		{"CUDA", tensor.CUDA},
		{"Vulkan", tensor.Vulkan},
		{"Metal", tensor.Metal},
		{"WebGPU", tensor.WebGPU},
	}

	for _, d := range devices {
		t.Run(d.name, func(t *testing.T) {
			// Verify String() method works.
			str := d.device.String()
			if str == "" || str == "Unknown" {
				t.Errorf("Device.String() = %q, want non-empty known device name", str)
			}
		})
	}
}

// TestDataTypeConstants verifies all data type constants are accessible.
func TestDataTypeConstants(t *testing.T) {
	dtypes := []struct {
		name  string
		dtype tensor.DataType
	}{
		{"Float32", tensor.Float32},
		{"Float64", tensor.Float64},
		{"Int32", tensor.Int32},
		{"Int64", tensor.Int64},
		{"Uint8", tensor.Uint8},
		{"Bool", tensor.Bool},
	}

	for _, dt := range dtypes {
		t.Run(dt.name, func(t *testing.T) {
			// Verify String() method works.
			str := dt.dtype.String()
			if str == "" {
				t.Errorf("DataType.String() = %q, want non-empty", str)
			}

			// Verify Size() method works.
			size := dt.dtype.Size()
			if size <= 0 {
				t.Errorf("DataType.Size() = %d, want > 0", size)
			}
		})
	}
}

// TestShapeAPI verifies Shape type alias exposes expected API.
func TestShapeAPI(t *testing.T) {
	shape := tensor.Shape{2, 3, 4}

	// Test NumElements.
	if n := shape.NumElements(); n != 24 {
		t.Errorf("NumElements() = %d, want 24", n)
	}

	// Test length (rank).
	if len(shape) != 3 {
		t.Errorf("len(shape) = %d, want 3", len(shape))
	}

	// Test Equal.
	if !shape.Equal(tensor.Shape{2, 3, 4}) {
		t.Error("Equal() = false, want true for identical shapes")
	}

	// Test Clone.
	clone := shape.Clone()
	if !clone.Equal(shape) {
		t.Error("Clone() created non-equal shape")
	}

	// Verify modifying clone doesn't affect original.
	clone[0] = 999
	if shape[0] == 999 {
		t.Error("Clone() didn't create independent copy")
	}
}

// TestBroadcastShapes verifies BroadcastShapes utility function.
func TestBroadcastShapes(t *testing.T) {
	tests := []struct {
		name          string
		shapeA        tensor.Shape
		shapeB        tensor.Shape
		wantShape     tensor.Shape
		wantBroadcast bool
		wantErr       bool
	}{
		{
			name:          "same shape",
			shapeA:        tensor.Shape{2, 3},
			shapeB:        tensor.Shape{2, 3},
			wantShape:     tensor.Shape{2, 3},
			wantBroadcast: false,
			wantErr:       false,
		},
		{
			name:          "broadcast scalar",
			shapeA:        tensor.Shape{2, 3},
			shapeB:        tensor.Shape{1},
			wantShape:     tensor.Shape{2, 3},
			wantBroadcast: true,
			wantErr:       false,
		},
		{
			name:          "broadcast dimension",
			shapeA:        tensor.Shape{3, 1},
			shapeB:        tensor.Shape{3, 4},
			wantShape:     tensor.Shape{3, 4},
			wantBroadcast: true,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotShape, gotBroadcast, err := tensor.BroadcastShapes(tt.shapeA, tt.shapeB)

			if (err != nil) != tt.wantErr {
				t.Errorf("BroadcastShapes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if !gotShape.Equal(tt.wantShape) {
					t.Errorf("BroadcastShapes() shape = %v, want %v", gotShape, tt.wantShape)
				}
				if gotBroadcast != tt.wantBroadcast {
					t.Errorf("BroadcastShapes() broadcast = %v, want %v", gotBroadcast, tt.wantBroadcast)
				}
			}
		})
	}
}

// TestManipulationFunctions verifies manipulation utility functions.
func TestManipulationFunctions(t *testing.T) {
	backend := cpu.New()

	t.Run("Cat", func(t *testing.T) {
		a := tensor.Ones[float32](tensor.Shape{2, 3}, backend)
		b := tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
		c := tensor.Cat([]*tensor.Tensor[float32, *cpu.CPUBackend]{a, b}, 0)

		if c == nil {
			t.Error("Cat() returned nil")
		}

		wantShape := tensor.Shape{4, 3}
		if !c.Shape().Equal(wantShape) {
			t.Errorf("Cat() shape = %v, want %v", c.Shape(), wantShape)
		}
	})

	t.Run("Where", func(t *testing.T) {
		cond := tensor.Full[bool](tensor.Shape{3}, true, backend)
		x := tensor.Full[float32](tensor.Shape{3}, 1.0, backend)
		y := tensor.Full[float32](tensor.Shape{3}, 0.0, backend)
		result := tensor.Where(cond, x, y)

		if result == nil {
			t.Error("Where() returned nil")
		}

		data := result.Data()
		for i, v := range data {
			if v != 1.0 {
				t.Errorf("Where() data[%d] = %v, want 1.0", i, v)
			}
		}
	})
}
