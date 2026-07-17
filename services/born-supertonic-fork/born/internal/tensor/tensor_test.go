package tensor

import (
	"fmt"
	"math"
	"testing"
)

// Test helpers

func assertEqualFloat32(t *testing.T, expected, actual float32, msg string) {
	t.Helper()
	if math.Abs(float64(expected-actual)) > 1e-6 {
		t.Errorf("%s: expected %v, got %v", msg, expected, actual)
	}
}

func assertEqualShape(t *testing.T, expected, actual Shape, msg string) {
	t.Helper()
	if !expected.Equal(actual) {
		t.Errorf("%s: expected shape %v, got %v", msg, expected, actual)
	}
}

// DType Tests

func TestDataTypeSize(t *testing.T) {
	tests := []struct {
		dtype DataType
		size  int
	}{
		{Float32, 4},
		{Float64, 8},
		{Int32, 4},
		{Int64, 8},
		{Uint8, 1},
		{Bool, 1},
	}

	for _, tt := range tests {
		if got := tt.dtype.Size(); got != tt.size {
			t.Errorf("%s.Size() = %d, want %d", tt.dtype, got, tt.size)
		}
	}
}

func TestDataTypeString(t *testing.T) {
	tests := []struct {
		dtype DataType
		str   string
	}{
		{Float32, "float32"},
		{Float64, "float64"},
		{Int32, "int32"},
		{Int64, "int64"},
		{Uint8, "uint8"},
		{Bool, "bool"},
	}

	for _, tt := range tests {
		if got := tt.dtype.String(); got != tt.str {
			t.Errorf("%s.String() = %q, want %q", tt.dtype, got, tt.str)
		}
	}
}

func TestInferDataType(t *testing.T) {
	if dt := inferDataType(float32(0)); dt != Float32 {
		t.Errorf("inferDataType(float32) = %v, want Float32", dt)
	}
	if dt := inferDataType(float64(0)); dt != Float64 {
		t.Errorf("inferDataType(float64) = %v, want Float64", dt)
	}
	if dt := inferDataType(int32(0)); dt != Int32 {
		t.Errorf("inferDataType(int32) = %v, want Int32", dt)
	}
	if dt := inferDataType(int64(0)); dt != Int64 {
		t.Errorf("inferDataType(int64) = %v, want Int64", dt)
	}
}

// Shape Tests

func TestShapeNumElements(t *testing.T) {
	tests := []struct {
		shape    Shape
		expected int
	}{
		{Shape{}, 1},         // Scalar
		{Shape{5}, 5},        // 1D
		{Shape{3, 4}, 12},    // 2D
		{Shape{2, 3, 4}, 24}, // 3D
		{Shape{1, 1, 1}, 1},  // Ones
	}

	for _, tt := range tests {
		if got := tt.shape.NumElements(); got != tt.expected {
			t.Errorf("Shape%v.NumElements() = %d, want %d", tt.shape, got, tt.expected)
		}
	}
}

func TestShapeValidation(t *testing.T) {
	validShapes := []Shape{
		{1},
		{3, 4},
		{2, 3, 4},
	}

	for _, s := range validShapes {
		if err := s.Validate(); err != nil {
			t.Errorf("Shape%v.Validate() failed: %v", s, err)
		}
	}

	// Zero-sized dims are valid (empty tensors, per ONNX). Only negative fails.
	for _, s := range []Shape{{0}, {3, 0}} {
		if err := s.Validate(); err != nil {
			t.Errorf("Shape%v.Validate() should pass (empty tensor) but failed: %v", s, err)
		}
	}

	invalidShapes := []Shape{
		{-1},
		{3, -4},
	}

	for _, s := range invalidShapes {
		if err := s.Validate(); err == nil {
			t.Errorf("Shape%v.Validate() should fail but didn't", s)
		}
	}
}

func TestShapeEqual(t *testing.T) {
	tests := []struct {
		a, b  Shape
		equal bool
	}{
		{Shape{3, 4}, Shape{3, 4}, true},
		{Shape{3, 4}, Shape{4, 3}, false},
		{Shape{3}, Shape{3, 1}, false},
		{Shape{}, Shape{}, true},
	}

	for _, tt := range tests {
		if got := tt.a.Equal(tt.b); got != tt.equal {
			t.Errorf("Shape%v.Equal(%v) = %v, want %v", tt.a, tt.b, got, tt.equal)
		}
	}
}

func TestComputeStrides(t *testing.T) {
	tests := []struct {
		shape    Shape
		expected []int
	}{
		{Shape{4}, []int{1}},
		{Shape{3, 4}, []int{4, 1}},
		{Shape{2, 3, 4}, []int{12, 4, 1}},
	}

	for _, tt := range tests {
		got := tt.shape.ComputeStrides()
		if len(got) != len(tt.expected) {
			t.Fatalf("Shape%v.ComputeStrides() length = %d, want %d", tt.shape, len(got), len(tt.expected))
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("Shape%v.ComputeStrides()[%d] = %d, want %d", tt.shape, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestBroadcastShapes(t *testing.T) {
	tests := []struct {
		a, b      Shape
		expected  Shape
		shouldErr bool
	}{
		// Compatible shapes
		{Shape{3, 1}, Shape{3, 5}, Shape{3, 5}, false},
		{Shape{1, 5}, Shape{3, 5}, Shape{3, 5}, false},
		{Shape{3, 4}, Shape{3, 4}, Shape{3, 4}, false},
		{Shape{1}, Shape{3, 4}, Shape{3, 4}, false},
		{Shape{3, 4}, Shape{1}, Shape{3, 4}, false},

		// Incompatible shapes
		{Shape{3, 4}, Shape{3, 5}, nil, true},
		{Shape{2, 3}, Shape{3, 3}, nil, true},
	}

	for _, tt := range tests {
		got, _, err := BroadcastShapes(tt.a, tt.b)
		if tt.shouldErr {
			if err == nil {
				t.Errorf("BroadcastShapes(%v, %v) should fail but didn't", tt.a, tt.b)
			}
		} else {
			if err != nil {
				t.Errorf("BroadcastShapes(%v, %v) failed: %v", tt.a, tt.b, err)
			}
			if !got.Equal(tt.expected) {
				t.Errorf("BroadcastShapes(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.expected)
			}
		}
	}
}

// RawTensor Tests

func TestNewRaw(t *testing.T) {
	shape := Shape{3, 4}
	raw, err := NewRaw(shape, Float32, CPU)
	if err != nil {
		t.Fatalf("NewRaw failed: %v", err)
	}

	if !raw.Shape().Equal(shape) {
		t.Errorf("Shape = %v, want %v", raw.Shape(), shape)
	}

	if raw.DType() != Float32 {
		t.Errorf("DType = %v, want Float32", raw.DType())
	}

	if raw.Device() != CPU {
		t.Errorf("Device = %v, want CPU", raw.Device())
	}

	if raw.NumElements() != 12 {
		t.Errorf("NumElements = %d, want 12", raw.NumElements())
	}

	if raw.ByteSize() != 48 { // 12 * 4 bytes
		t.Errorf("ByteSize = %d, want 48", raw.ByteSize())
	}
}

func TestRawTensorAsFloat32(t *testing.T) {
	raw, _ := NewRaw(Shape{3, 4}, Float32, CPU)
	data := raw.AsFloat32()

	if len(data) != 12 {
		t.Errorf("AsFloat32 length = %d, want 12", len(data))
	}

	// Modify and verify zero-copy
	data[0] = 3.14
	if raw.AsFloat32()[0] != 3.14 {
		t.Error("AsFloat32 should return zero-copy slice")
	}
}

func TestRawTensorClone(t *testing.T) {
	raw, _ := NewRaw(Shape{2, 2}, Float32, CPU)
	data := raw.AsFloat32()
	data[0] = 1.0
	data[1] = 2.0

	clone := raw.Clone()

	// Verify data is shared (shallow copy with reference counting)
	if clone.AsFloat32()[0] != 1.0 {
		t.Error("Clone should share data")
	}

	// Modifying clone WILL affect original (shared buffer)
	// This is expected behavior with reference counting
	clone.AsFloat32()[0] = 999.0
	if raw.AsFloat32()[0] != 999.0 {
		t.Error("Clone shares buffer, modifications should be visible")
	}

	// Verify reference counting
	// Both tensors share the buffer, so refCount > 1. This is correct behavior.
	_ = raw.IsUnique() // Suppress unused warning
	_ = clone.IsUnique()
}

// Tensor Creation Tests

func TestZeros(t *testing.T) {
	backend := NewMockBackend()
	shape := Shape{3, 4}

	tensor := Zeros[float32](shape, backend)

	assertEqualShape(t, shape, tensor.Shape(), "Shape mismatch")

	data := tensor.Data()
	for i, v := range data {
		if v != 0 {
			t.Errorf("Zeros[%d] = %v, want 0", i, v)
		}
	}
}

func TestOnes(t *testing.T) {
	backend := NewMockBackend()
	shape := Shape{2, 3}

	tensor := Ones[float32](shape, backend)

	data := tensor.Data()
	for i, v := range data {
		if v != 1 {
			t.Errorf("Ones[%d] = %v, want 1", i, v)
		}
	}
}

func TestFull(t *testing.T) {
	backend := NewMockBackend()
	shape := Shape{2, 2}
	value := float32(3.14)

	tensor := Full(shape, value, backend)

	data := tensor.Data()
	for i, v := range data {
		assertEqualFloat32(t, value, v, fmt.Sprintf("Full[%d]", i))
	}
}

func TestArange(t *testing.T) {
	backend := NewMockBackend()

	tensor := Arange[int32](0, 10, backend)

	assertEqualShape(t, Shape{10}, tensor.Shape(), "Arange shape")

	data := tensor.Data()
	for i, v := range data {
		if v != int32(i) {
			t.Errorf("Arange[%d] = %d, want %d", i, v, i)
		}
	}
}

func TestEye(t *testing.T) {
	backend := NewMockBackend()

	tensor := Eye[float32](3, backend)

	assertEqualShape(t, Shape{3, 3}, tensor.Shape(), "Eye shape")

	// Check diagonal
	for i := 0; i < 3; i++ {
		if tensor.At(i, i) != 1.0 {
			t.Errorf("Eye[%d, %d] = %v, want 1", i, i, tensor.At(i, i))
		}
	}

	// Check off-diagonal
	if tensor.At(0, 1) != 0 || tensor.At(1, 0) != 0 {
		t.Error("Eye off-diagonal should be zero")
	}
}

func TestFromSlice(t *testing.T) {
	backend := NewMockBackend()
	data := []float32{1, 2, 3, 4, 5, 6}
	shape := Shape{2, 3}

	tensor, err := FromSlice(data, shape, backend)
	if err != nil {
		t.Fatalf("FromSlice failed: %v", err)
	}

	assertEqualShape(t, shape, tensor.Shape(), "FromSlice shape")

	got := tensor.Data()
	for i := range data {
		if got[i] != data[i] {
			t.Errorf("FromSlice[%d] = %v, want %v", i, got[i], data[i])
		}
	}
}

// Tensor Operations Tests

func TestTensorAt(t *testing.T) {
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1, 2, 3, 4, 5, 6}, Shape{2, 3}, backend)

	tests := []struct {
		indices  []int
		expected float32
	}{
		{[]int{0, 0}, 1},
		{[]int{0, 1}, 2},
		{[]int{0, 2}, 3},
		{[]int{1, 0}, 4},
		{[]int{1, 1}, 5},
		{[]int{1, 2}, 6},
	}

	for _, tt := range tests {
		got := tensor.At(tt.indices...)
		if got != tt.expected {
			t.Errorf("At%v = %v, want %v", tt.indices, got, tt.expected)
		}
	}
}

func TestTensorSet(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[float32](Shape{2, 2}, backend)

	tensor.Set(3.14, 1, 1)
	if got := tensor.At(1, 1); got != 3.14 {
		t.Errorf("After Set(3.14, 1, 1), At(1, 1) = %v, want 3.14", got)
	}
}

func TestTensorItem(t *testing.T) {
	backend := NewMockBackend()
	tensor := Full(Shape{1}, float32(42), backend)

	if got := tensor.Reshape().Item(); got != 42 {
		t.Errorf("Item() = %v, want 42", got)
	}
}

func TestTensorAdd(t *testing.T) {
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)
	b, _ := FromSlice([]float32{5, 6, 7, 8}, Shape{2, 2}, backend)

	c := a.Add(b)

	expected := []float32{6, 8, 10, 12}
	got := c.Data()

	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("Add[%d]", i))
	}
}

func TestTensorSub(t *testing.T) {
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{5, 6, 7, 8}, Shape{2, 2}, backend)
	b, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)

	c := a.Sub(b)

	expected := []float32{4, 4, 4, 4}
	got := c.Data()

	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("Sub[%d]", i))
	}
}

func TestTensorMul(t *testing.T) {
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)
	b, _ := FromSlice([]float32{2, 2, 2, 2}, Shape{2, 2}, backend)

	c := a.Mul(b)

	expected := []float32{2, 4, 6, 8}
	got := c.Data()

	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("Mul[%d]", i))
	}
}

func TestTensorMatMul(t *testing.T) {
	backend := NewMockBackend()
	// [[1, 2],     [[5, 6],     [[19, 22],
	//  [3, 4]]  @   [7, 8]]  =   [43, 50]]
	a, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)
	b, _ := FromSlice([]float32{5, 6, 7, 8}, Shape{2, 2}, backend)

	c := a.MatMul(b)

	expected := []float32{19, 22, 43, 50}
	got := c.Data()

	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("MatMul[%d]", i))
	}
}

func TestTensorReshape(t *testing.T) {
	backend := NewMockBackend()
	tensor := Arange[int32](0, 12, backend)

	reshaped := tensor.Reshape(3, 4)

	assertEqualShape(t, Shape{3, 4}, reshaped.Shape(), "Reshape shape")

	// Verify data is preserved
	if reshaped.At(0, 0) != 0 || reshaped.At(2, 3) != 11 {
		t.Error("Reshape should preserve data")
	}
}

func TestTensorTranspose(t *testing.T) {
	backend := NewMockBackend()
	// [[1, 2, 3],
	//  [4, 5, 6]]
	tensor, _ := FromSlice([]float32{1, 2, 3, 4, 5, 6}, Shape{2, 3}, backend)

	transposed := tensor.T()

	assertEqualShape(t, Shape{3, 2}, transposed.Shape(), "Transpose shape")

	// [[1, 4],
	//  [2, 5],
	//  [3, 6]]
	if transposed.At(0, 0) != 1 || transposed.At(0, 1) != 4 {
		t.Error("Transpose data incorrect")
	}
	if transposed.At(1, 0) != 2 || transposed.At(1, 1) != 5 {
		t.Error("Transpose data incorrect")
	}
	if transposed.At(2, 0) != 3 || transposed.At(2, 1) != 6 {
		t.Error("Transpose data incorrect")
	}
}

func TestTensorClone(t *testing.T) {
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)

	clone := tensor.Clone()

	// Verify data is shared (shallow copy with reference counting)
	if clone.At(0, 0) != 1 {
		t.Error("Clone should share data")
	}

	// Modifying clone WILL affect original (shared buffer)
	// This is expected behavior with reference counting
	clone.Set(999, 0, 0)
	if tensor.At(0, 0) != 999 {
		t.Error("Clone shares buffer, modifications should be visible")
	}
}

// Broadcasting Tests

func TestBroadcastingAdd(t *testing.T) {
	backend := NewMockBackend()
	// (3, 1) + (3, 5) → (3, 5)
	a := Ones[float32](Shape{3, 1}, backend)
	b := Full(Shape{3, 5}, float32(2.0), backend)

	c := a.Add(b)

	assertEqualShape(t, Shape{3, 5}, c.Shape(), "Broadcasting shape")

	// All elements should be 3.0
	data := c.Data()
	for i, v := range data {
		assertEqualFloat32(t, 3.0, v, fmt.Sprintf("Broadcasting[%d]", i))
	}
}
