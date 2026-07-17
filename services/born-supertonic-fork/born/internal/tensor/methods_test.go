package tensor

import (
	"testing"
)

// Tensor Methods Tests

func TestTensorDType(t *testing.T) {
	backend := NewMockBackend()

	t32 := Zeros[float32](Shape{2, 2}, backend)
	if t32.DType() != Float32 {
		t.Errorf("DType() = %v, want Float32", t32.DType())
	}

	t64 := Zeros[float64](Shape{2, 2}, backend)
	if t64.DType() != Float64 {
		t.Errorf("DType() = %v, want Float64", t64.DType())
	}

	ti32 := Zeros[int32](Shape{2, 2}, backend)
	if ti32.DType() != Int32 {
		t.Errorf("DType() = %v, want Int32", ti32.DType())
	}

	ti64 := Zeros[int64](Shape{2, 2}, backend)
	if ti64.DType() != Int64 {
		t.Errorf("DType() = %v, want Int64", ti64.DType())
	}

	tu8 := Zeros[uint8](Shape{2, 2}, backend)
	if tu8.DType() != Uint8 {
		t.Errorf("DType() = %v, want Uint8", tu8.DType())
	}

	tb := Zeros[bool](Shape{2, 2}, backend)
	if tb.DType() != Bool {
		t.Errorf("DType() = %v, want Bool", tb.DType())
	}
}

func TestTensorDevice(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[float32](Shape{2, 2}, backend)
	if tensor.Device() != CPU {
		t.Errorf("Device() = %v, want CPU", tensor.Device())
	}
}

func TestTensorRaw(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[float32](Shape{2, 2}, backend)
	raw := tensor.Raw()
	if raw == nil {
		t.Error("Raw() should not return nil")
	}
	if !raw.Shape().Equal(Shape{2, 2}) {
		t.Errorf("Raw().Shape() = %v, want {2, 2}", raw.Shape())
	}
	if raw.DType() != Float32 {
		t.Errorf("Raw().DType() = %v, want Float32", raw.DType())
	}
}

func TestTensorBackend(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[float32](Shape{2, 2}, backend)
	if tensor.Backend() != backend {
		t.Error("Backend() should return the same backend instance")
	}
	if tensor.Backend().Name() != "mock" {
		t.Errorf("Backend().Name() = %v, want mock", tensor.Backend().Name())
	}
}

func TestTensorGrad(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[float32](Shape{2, 2}, backend)

	// Initially no gradient
	if grad := tensor.Grad(); grad != nil {
		t.Error("Grad() should return nil initially")
	}

	// Set gradient
	gradTensor := Ones[float32](Shape{2, 2}, backend)
	tensor.SetGrad(gradTensor)

	if grad := tensor.Grad(); grad == nil {
		t.Error("Grad() should not be nil after SetGrad")
	} else {
		if !grad.Shape().Equal(Shape{2, 2}) {
			t.Errorf("Grad().Shape() = %v, want {2, 2}", grad.Shape())
		}
		// Check gradient values
		gradData := grad.Data()
		for i, v := range gradData {
			if v != 1 {
				t.Errorf("Grad().Data()[%d] = %v, want 1", i, v)
			}
		}
	}
}

func TestTensorSetGradNil(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[float32](Shape{2, 2}, backend)

	// Set gradient
	gradTensor := Ones[float32](Shape{2, 2}, backend)
	tensor.SetGrad(gradTensor)

	if tensor.Grad() == nil {
		t.Error("Grad() should not be nil after SetGrad")
	}

	// Set to nil
	tensor.SetGrad(nil)

	if tensor.Grad() != nil {
		t.Error("Grad() should be nil after SetGrad(nil)")
	}
}

func TestTensorDetach(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[float32](Shape{2, 2}, backend)
	gradTensor := Ones[float32](Shape{2, 2}, backend)
	tensor.SetGrad(gradTensor)

	detached := tensor.Detach()

	// Detached tensor should have no gradient
	if detached.Grad() != nil {
		t.Error("Detach() should create tensor without gradient")
	}

	// Original tensor should still have gradient
	if tensor.Grad() == nil {
		t.Error("Detach() should not modify original tensor gradient")
	}

	// Detached tensor should have same data
	if !detached.Shape().Equal(tensor.Shape()) {
		t.Error("Detach() should preserve shape")
	}

	tensorData := tensor.Data()
	detachedData := detached.Data()
	for i := range tensorData {
		if tensorData[i] != detachedData[i] {
			t.Errorf("Detach() data[%d] = %v, want %v", i, detachedData[i], tensorData[i])
		}
	}
}

func TestTensorString(t *testing.T) {
	backend := NewMockBackend()

	// Test with small tensor
	tensor, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)
	str := tensor.String()
	if str == "" {
		t.Error("String() should not return empty string")
	}
	// Should contain tensor info
	if len(str) < 10 {
		t.Errorf("String() = %q, seems too short", str)
	}

	// Test with different types
	intTensor := Zeros[int32](Shape{2, 2}, backend)
	intStr := intTensor.String()
	if intStr == "" {
		t.Error("String() for int32 should not return empty string")
	}

	boolTensor := Zeros[bool](Shape{2, 2}, backend)
	boolStr := boolTensor.String()
	if boolStr == "" {
		t.Error("String() for bool should not return empty string")
	}
}

func TestTensorRequireGrad(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[float32](Shape{2, 2}, backend)

	// Initially should not require grad
	if tensor.RequiresGrad() {
		t.Error("RequiresGrad() should return false initially")
	}

	// After RequireGrad
	tensor.RequireGrad()
	if !tensor.RequiresGrad() {
		t.Error("RequiresGrad() should return true after RequireGrad()")
	}
}

func TestTensorRequiresGradMultipleCalls(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[float32](Shape{2, 2}, backend)

	// Multiple calls should be safe
	tensor.RequireGrad()
	tensor.RequireGrad()
	tensor.RequireGrad()

	if !tensor.RequiresGrad() {
		t.Error("RequiresGrad() should return true after multiple RequireGrad() calls")
	}
}

// Data Access Tests

func TestTensorDataFloat64(t *testing.T) {
	backend := NewMockBackend()
	data := []float64{1.5, 2.5, 3.5, 4.5}
	tensor, _ := FromSlice(data, Shape{2, 2}, backend)

	got := tensor.Data()
	for i, exp := range data {
		if got[i] != exp {
			t.Errorf("Data()[%d] = %v, want %v", i, got[i], exp)
		}
	}
}

func TestTensorDataInt64(t *testing.T) {
	backend := NewMockBackend()
	data := []int64{1, 2, 3, 4}
	tensor, _ := FromSlice(data, Shape{2, 2}, backend)

	got := tensor.Data()
	for i, exp := range data {
		if got[i] != exp {
			t.Errorf("Data()[%d] = %v, want %v", i, got[i], exp)
		}
	}
}

func TestTensorDataUint8(t *testing.T) {
	backend := NewMockBackend()
	data := []uint8{1, 2, 3, 4}
	tensor, _ := FromSlice(data, Shape{2, 2}, backend)

	got := tensor.Data()
	for i, exp := range data {
		if got[i] != exp {
			t.Errorf("Data()[%d] = %v, want %v", i, got[i], exp)
		}
	}
}

func TestTensorDataBool(t *testing.T) {
	backend := NewMockBackend()
	data := []bool{true, false, true, false}
	tensor, _ := FromSlice(data, Shape{2, 2}, backend)

	got := tensor.Data()
	for i, exp := range data {
		if got[i] != exp {
			t.Errorf("Data()[%d] = %v, want %v", i, got[i], exp)
		}
	}
}

// Item Tests for different types

func TestTensorItemInt32(t *testing.T) {
	backend := NewMockBackend()
	tensor := Full(Shape{1}, int32(42), backend)

	if got := tensor.Reshape().Item(); got != 42 {
		t.Errorf("Item() = %v, want 42", got)
	}
}

func TestTensorItemFloat64(t *testing.T) {
	backend := NewMockBackend()
	tensor := Full(Shape{1}, float64(3.14), backend)

	if got := tensor.Reshape().Item(); got != 3.14 {
		t.Errorf("Item() = %v, want 3.14", got)
	}
}

func TestTensorItemBool(t *testing.T) {
	backend := NewMockBackend()
	tensor := Full(Shape{1}, true, backend)

	if got := tensor.Reshape().Item(); got != true {
		t.Errorf("Item() = %v, want true", got)
	}
}

// At and Set Tests for different types

func TestTensorAtSetInt64(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[int64](Shape{2, 2}, backend)

	tensor.Set(int64(123), 1, 1)
	if got := tensor.At(1, 1); got != int64(123) {
		t.Errorf("After Set(123, 1, 1), At(1, 1) = %v, want 123", got)
	}
}

func TestTensorAtSetUint8(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[uint8](Shape{2, 2}, backend)

	tensor.Set(uint8(255), 0, 1)
	if got := tensor.At(0, 1); got != uint8(255) {
		t.Errorf("After Set(255, 0, 1), At(0, 1) = %v, want 255", got)
	}
}

func TestTensorAtSetBool(t *testing.T) {
	backend := NewMockBackend()
	tensor := Zeros[bool](Shape{2, 2}, backend)

	tensor.Set(true, 1, 0)
	if got := tensor.At(1, 0); got != true {
		t.Errorf("After Set(true, 1, 0), At(1, 0) = %v, want true", got)
	}
}
