package tensor

import (
	"testing"
)

// RawTensor Tests

func TestRawTensorAsInt64(t *testing.T) {
	raw, _ := NewRaw(Shape{3, 2}, Int64, CPU)
	data := raw.AsInt64()

	if len(data) != 6 {
		t.Errorf("AsInt64 length = %d, want 6", len(data))
	}

	// Modify and verify zero-copy
	data[0] = 42
	if raw.AsInt64()[0] != 42 {
		t.Error("AsInt64 should return zero-copy slice")
	}
}

func TestRawTensorAsUint8(t *testing.T) {
	raw, _ := NewRaw(Shape{4, 4}, Uint8, CPU)
	data := raw.AsUint8()

	if len(data) != 16 {
		t.Errorf("AsUint8 length = %d, want 16", len(data))
	}

	// Modify and verify zero-copy
	data[0] = 255
	if raw.AsUint8()[0] != 255 {
		t.Error("AsUint8 should return zero-copy slice")
	}
}

func TestRawTensorAsBool(t *testing.T) {
	raw, _ := NewRaw(Shape{2, 2}, Bool, CPU)
	data := raw.AsBool()

	if len(data) != 4 {
		t.Errorf("AsBool length = %d, want 4", len(data))
	}

	// Modify and verify zero-copy
	data[0] = true
	if raw.AsBool()[0] != true {
		t.Error("AsBool should return zero-copy slice")
	}
}

func TestRawTensorRelease(_ *testing.T) {
	raw, _ := NewRaw(Shape{2, 2}, Float32, CPU)

	// Should not panic
	raw.Release()

	// Multiple releases should be safe (reference counting)
	raw.Release()
}

func TestRawTensorForceNonUnique(t *testing.T) {
	raw, _ := NewRaw(Shape{2, 2}, Float32, CPU)

	// Initially should be unique (refCount = 1)
	if !raw.IsUnique() {
		t.Error("New RawTensor should be unique initially")
	}

	// ForceNonUnique should make it non-unique
	raw.ForceNonUnique()

	if raw.IsUnique() {
		t.Error("After ForceNonUnique(), IsUnique() should return false")
	}
}

// RawTensor Copy-on-Write Tests

func TestRawTensorCloneIsShared(t *testing.T) {
	raw, _ := NewRaw(Shape{2, 2}, Float32, CPU)
	data := raw.AsFloat32()
	data[0] = 1.0

	clone := raw.Clone()

	// Both should share the buffer
	if clone.AsFloat32()[0] != 1.0 {
		t.Error("Clone should share data initially")
	}

	// Neither should be unique (refCount > 1)
	if raw.IsUnique() || clone.IsUnique() {
		t.Error("After Clone(), neither tensor should be unique")
	}
}

// RawTensor Different Types Tests

func TestNewRawAllTypes(t *testing.T) {
	types := []struct {
		dtype       DataType
		elementSize int
	}{
		{Float32, 4},
		{Float64, 8},
		{Int32, 4},
		{Int64, 8},
		{Uint8, 1},
		{Bool, 1},
	}

	shape := Shape{2, 3}
	for _, tt := range types {
		raw, err := NewRaw(shape, tt.dtype, CPU)
		if err != nil {
			t.Fatalf("NewRaw(%v, %v) failed: %v", shape, tt.dtype, err)
		}

		if raw.DType() != tt.dtype {
			t.Errorf("DType = %v, want %v", raw.DType(), tt.dtype)
		}

		expectedByteSize := 6 * tt.elementSize // 2*3 elements
		if raw.ByteSize() != expectedByteSize {
			t.Errorf("ByteSize = %d, want %d for type %v", raw.ByteSize(), expectedByteSize, tt.dtype)
		}
	}
}

// RawTensor Invalid Creation Tests

func TestNewRawInvalidShape(t *testing.T) {
	// Zero-sized dims allocate empty tensors (ONNX allows them); only negative fails.
	for _, shape := range []Shape{{0}, {2, 0}} {
		if _, err := NewRaw(shape, Float32, CPU); err != nil {
			t.Errorf("NewRaw(%v) should allocate an empty tensor but failed: %v", shape, err)
		}
	}

	invalidShapes := []Shape{
		{-1},
		{2, -3},
	}

	for _, shape := range invalidShapes {
		_, err := NewRaw(shape, Float32, CPU)
		if err == nil {
			t.Errorf("NewRaw(%v) should fail but didn't", shape)
		}
	}
}

// Test reference counting behavior

func TestRawTensorReferenceCounting(t *testing.T) {
	raw, _ := NewRaw(Shape{2, 2}, Float32, CPU)

	// Initially unique
	if !raw.IsUnique() {
		t.Error("New tensor should be unique")
	}

	// Clone increases refCount
	clone1 := raw.Clone()
	if raw.IsUnique() || clone1.IsUnique() {
		t.Error("After Clone(), neither tensor should be unique")
	}

	// Another clone
	clone2 := raw.Clone()
	if raw.IsUnique() || clone1.IsUnique() || clone2.IsUnique() {
		t.Error("With 3 references, none should be unique")
	}

	// Release clones
	clone1.Release()
	clone2.Release()

	// After releasing clones, original might become unique again
	// (depending on implementation details)
	// This is implementation-specific, so we just check it doesn't panic
	_ = raw.IsUnique()
}

// Test As* methods panic on wrong type

func TestRawTensorAsWrongTypePanics(t *testing.T) {
	// Float32 tensor
	raw32, _ := NewRaw(Shape{2}, Float32, CPU)

	// AsFloat32 should work
	_ = raw32.AsFloat32()

	// AsFloat64 should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("AsFloat64 on Float32 tensor should panic")
		}
	}()
	_ = raw32.AsFloat64()
}

func TestRawTensorAsInt32WrongTypePanics(t *testing.T) {
	raw, _ := NewRaw(Shape{2}, Float32, CPU)

	defer func() {
		if r := recover(); r == nil {
			t.Error("AsInt32 on Float32 tensor should panic")
		}
	}()
	_ = raw.AsInt32()
}

func TestRawTensorAsInt64WrongTypePanics(t *testing.T) {
	raw, _ := NewRaw(Shape{2}, Float32, CPU)

	defer func() {
		if r := recover(); r == nil {
			t.Error("AsInt64 on Float32 tensor should panic")
		}
	}()
	_ = raw.AsInt64()
}

func TestRawTensorAsUint8WrongTypePanics(t *testing.T) {
	raw, _ := NewRaw(Shape{2}, Float32, CPU)

	defer func() {
		if r := recover(); r == nil {
			t.Error("AsUint8 on Float32 tensor should panic")
		}
	}()
	_ = raw.AsUint8()
}

func TestRawTensorAsBoolWrongTypePanics(t *testing.T) {
	raw, _ := NewRaw(Shape{2}, Float32, CPU)

	defer func() {
		if r := recover(); r == nil {
			t.Error("AsBool on Float32 tensor should panic")
		}
	}()
	_ = raw.AsBool()
}

// Test empty tensor (scalar)

func TestRawTensorScalar(t *testing.T) {
	raw, _ := NewRaw(Shape{}, Float32, CPU)

	if raw.NumElements() != 1 {
		t.Errorf("Scalar tensor NumElements = %d, want 1", raw.NumElements())
	}

	if raw.ByteSize() != 4 {
		t.Errorf("Scalar tensor ByteSize = %d, want 4", raw.ByteSize())
	}

	data := raw.AsFloat32()
	if len(data) != 1 {
		t.Errorf("Scalar tensor data length = %d, want 1", len(data))
	}
}
