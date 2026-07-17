package cpu

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// Helper to create test backend.
func newTestBackend() *CPUBackend {
	return New()
}

// Helper to check float32 slices are equal within epsilon.
func float32SliceEqual(a, b []float32) bool {
	const epsilon = 1e-6
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		diff := a[i] - b[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > epsilon {
			return false
		}
	}
	return true
}

// TestCPUBackend_New tests backend creation.
func TestCPUBackend_New(t *testing.T) {
	backend := New()
	if backend == nil {
		t.Fatal("New() returned nil")
	}
	if backend.Name() != "CPU" {
		t.Errorf("Expected name 'CPU', got '%s'", backend.Name())
	}
	if backend.Device() != tensor.CPU {
		t.Errorf("Expected device CPU, got %v", backend.Device())
	}
}

// TestCPUBackend_Add tests element-wise addition.
func TestCPUBackend_Add(t *testing.T) {
	backend := newTestBackend()

	// Test same shape addition
	t.Run("SameShape", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		bData := b.AsFloat32()
		for i := range aData {
			aData[i] = float32(i + 1)  // 1, 2, 3, 4, 5, 6
			bData[i] = float32(i + 10) // 10, 11, 12, 13, 14, 15
		}

		result := backend.Add(a, b)

		// 1+10=11, 2+11=13, 3+12=15, 4+13=17, 5+14=19, 6+15=21
		expected := []float32{11, 13, 15, 17, 19, 21}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Add failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})

	// Test inplace optimization
	t.Run("InplaceOptimization", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		bData := b.AsFloat32()
		aData[0], aData[1], aData[2] = 1, 2, 3
		bData[0], bData[1], bData[2] = 10, 20, 30

		// a is unique (refCount == 1), should modify inplace
		if !a.IsUnique() {
			t.Skip("Test requires unique tensor for inplace path")
		}

		result := backend.Add(a, b)

		// Should return same pointer (inplace)
		if result != a {
			t.Log("Note: inplace optimization may not trigger (this is OK)")
		}

		expected := []float32{11, 22, 33}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Add with inplace failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})
}

// TestCPUBackend_AddBroadcasting tests broadcasting addition.
func TestCPUBackend_AddBroadcasting(t *testing.T) {
	backend := newTestBackend()

	// Test [3, 1] + [4] -> [3, 4]
	t.Run("Broadcast_3x1_plus_4", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3, 1}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		bData := b.AsFloat32()
		aData[0], aData[1], aData[2] = 1, 2, 3
		bData[0], bData[1], bData[2], bData[3] = 10, 20, 30, 40

		result := backend.Add(a, b)

		if !result.Shape().Equal(tensor.Shape{3, 4}) {
			t.Fatalf("Expected shape [3, 4], got %v", result.Shape())
		}

		// Expected:
		// [1 + 10, 1 + 20, 1 + 30, 1 + 40] = [11, 21, 31, 41]
		// [2 + 10, 2 + 20, 2 + 30, 2 + 40] = [12, 22, 32, 42]
		// [3 + 10, 3 + 20, 3 + 30, 3 + 40] = [13, 23, 33, 43]
		expected := []float32{11, 21, 31, 41, 12, 22, 32, 42, 13, 23, 33, 43}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Broadcasting add failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})

	// Test scalar broadcasting
	t.Run("ScalarBroadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		for i := range aData {
			aData[i] = float32(i + 1)
		}
		b.AsFloat32()[0] = 10

		result := backend.Add(a, b)

		expected := []float32{11, 12, 13, 14, 15, 16}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Scalar broadcast failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})
}

// TestCPUBackend_Sub tests subtraction.
func TestCPUBackend_Sub(t *testing.T) {
	backend := newTestBackend()

	a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
	b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

	aData := a.AsFloat32()
	bData := b.AsFloat32()
	aData[0], aData[1], aData[2] = 10, 20, 30
	bData[0], bData[1], bData[2] = 1, 2, 3

	result := backend.Sub(a, b)

	expected := []float32{9, 18, 27}
	if !float32SliceEqual(result.AsFloat32(), expected) {
		t.Errorf("Sub failed: got %v, expected %v", result.AsFloat32(), expected)
	}
}

// TestCPUBackend_Mul tests multiplication.
func TestCPUBackend_Mul(t *testing.T) {
	backend := newTestBackend()

	a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
	b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

	aData := a.AsFloat32()
	bData := b.AsFloat32()
	aData[0], aData[1], aData[2] = 2, 3, 4
	bData[0], bData[1], bData[2] = 10, 10, 10

	result := backend.Mul(a, b)

	expected := []float32{20, 30, 40}
	if !float32SliceEqual(result.AsFloat32(), expected) {
		t.Errorf("Mul failed: got %v, expected %v", result.AsFloat32(), expected)
	}
}

// TestCPUBackend_Div tests division.
func TestCPUBackend_Div(t *testing.T) {
	backend := newTestBackend()

	a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
	b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

	aData := a.AsFloat32()
	bData := b.AsFloat32()
	aData[0], aData[1], aData[2] = 20, 30, 40
	bData[0], bData[1], bData[2] = 2, 3, 4

	result := backend.Div(a, b)

	expected := []float32{10, 10, 10}
	if !float32SliceEqual(result.AsFloat32(), expected) {
		t.Errorf("Div failed: got %v, expected %v", result.AsFloat32(), expected)
	}
}

// TestCPUBackend_MatMul tests matrix multiplication.
func TestCPUBackend_MatMul(t *testing.T) {
	backend := newTestBackend()

	// Test 2x3 @ 3x2 -> 2x2
	t.Run("2x3_matmul_3x2", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3, 2}, tensor.Float32, tensor.CPU)

		// A = [[1, 2, 3],
		//      [4, 5, 6]]
		aData := a.AsFloat32()
		aData[0], aData[1], aData[2] = 1, 2, 3
		aData[3], aData[4], aData[5] = 4, 5, 6

		// B = [[1, 2],
		//      [3, 4],
		//      [5, 6]]
		bData := b.AsFloat32()
		bData[0], bData[1] = 1, 2
		bData[2], bData[3] = 3, 4
		bData[4], bData[5] = 5, 6

		result := backend.MatMul(a, b)

		if !result.Shape().Equal(tensor.Shape{2, 2}) {
			t.Fatalf("Expected shape [2, 2], got %v", result.Shape())
		}

		// Expected:
		// [1*1 + 2*3 + 3*5, 1*2 + 2*4 + 3*6] = [22, 28]
		// [4*1 + 5*3 + 6*5, 4*2 + 5*4 + 6*6] = [49, 64]
		expected := []float32{22, 28, 49, 64}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("MatMul failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})

	// Test identity matrix
	t.Run("IdentityMatrix", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, tensor.CPU)
		identity, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		aData[0], aData[1], aData[2], aData[3] = 1, 2, 3, 4

		idData := identity.AsFloat32()
		idData[0], idData[1], idData[2], idData[3] = 1, 0, 0, 1

		result := backend.MatMul(a, identity)

		expected := []float32{1, 2, 3, 4}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("MatMul with identity failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})
}

// TestCPUBackend_Reshape tests reshape operation.
func TestCPUBackend_Reshape(t *testing.T) {
	backend := newTestBackend()

	a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
	aData := a.AsFloat32()
	for i := range aData {
		aData[i] = float32(i + 1)
	}

	// Reshape to [3, 2]
	result := backend.Reshape(a, tensor.Shape{3, 2})

	if !result.Shape().Equal(tensor.Shape{3, 2}) {
		t.Fatalf("Expected shape [3, 2], got %v", result.Shape())
	}

	// Data should remain same (row-major order)
	expected := []float32{1, 2, 3, 4, 5, 6}
	if !float32SliceEqual(result.AsFloat32(), expected) {
		t.Errorf("Reshape failed: got %v, expected %v", result.AsFloat32(), expected)
	}
}

// TestCPUBackend_Transpose tests transpose operation.
func TestCPUBackend_Transpose(t *testing.T) {
	backend := newTestBackend()

	// Test 2x3 transpose
	t.Run("2x3_transpose", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
		aData := a.AsFloat32()
		// [[1, 2, 3],
		//  [4, 5, 6]]
		aData[0], aData[1], aData[2] = 1, 2, 3
		aData[3], aData[4], aData[5] = 4, 5, 6

		result := backend.Transpose(a)

		if !result.Shape().Equal(tensor.Shape{3, 2}) {
			t.Fatalf("Expected shape [3, 2], got %v", result.Shape())
		}

		// Expected:
		// [[1, 4],
		//  [2, 5],
		//  [3, 6]]
		expected := []float32{1, 4, 2, 5, 3, 6}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Transpose failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})

	// Test square matrix transpose
	t.Run("SquareMatrix", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3, 3}, tensor.Float32, tensor.CPU)
		aData := a.AsFloat32()
		for i := range aData {
			aData[i] = float32(i + 1)
		}

		result := backend.Transpose(a)

		// [[1, 2, 3],     [[1, 4, 7],
		//  [4, 5, 6],  ->  [2, 5, 8],
		//  [7, 8, 9]]      [3, 6, 9]]
		expected := []float32{1, 4, 7, 2, 5, 8, 3, 6, 9}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Square matrix transpose failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})
}

// TestCPUBackend_MultiDType tests operations with different data types.
func TestCPUBackend_MultiDType(t *testing.T) {
	backend := newTestBackend()

	// Test float64
	t.Run("Float64", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		aData[0], aData[1], aData[2] = 1.5, 2.5, 3.5
		bData[0], bData[1], bData[2] = 0.5, 0.5, 0.5

		result := backend.Add(a, b)

		expected := []float64{2.0, 3.0, 4.0}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 add failed at index %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	// Test int32
	t.Run("Int32", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		aData[0], aData[1], aData[2] = 10, 20, 30
		bData[0], bData[1], bData[2] = 1, 2, 3

		result := backend.Mul(a, b)

		expected := []int32{10, 40, 90}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 mul failed at index %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	// Test int64
	t.Run("Int64", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		aData[0], aData[1], aData[2], aData[3] = 1, 2, 3, 4
		bData[0], bData[1], bData[2], bData[3] = 1, 0, 0, 1

		result := backend.MatMul(a, b)

		expected := []int64{1, 2, 3, 4}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 matmul failed at index %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_ReferenceCountingIntegration tests reference counting with backend operations.
func TestCPUBackend_ReferenceCountingIntegration(t *testing.T) {
	backend := newTestBackend()

	a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
	aData := a.AsFloat32()
	aData[0], aData[1], aData[2] = 1, 2, 3

	// Clone creates shared buffer
	clone := a.Clone()

	// Both should share buffer
	b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
	bData := b.AsFloat32()
	bData[0], bData[1], bData[2] = 10, 20, 30

	// Add should create new tensor (refCount > 1)
	result := backend.Add(a, b)

	// Verify result is correct
	expected := []float32{11, 22, 33}
	if !float32SliceEqual(result.AsFloat32(), expected) {
		t.Errorf("Add with shared buffer failed: got %v, expected %v", result.AsFloat32(), expected)
	}

	// Original tensors should be unchanged
	aData = a.AsFloat32()
	if aData[0] != 1 || aData[1] != 2 || aData[2] != 3 {
		t.Errorf("Original tensor a was modified: %v", aData)
	}

	cloneData := clone.AsFloat32()
	if cloneData[0] != 1 || cloneData[1] != 2 || cloneData[2] != 3 {
		t.Errorf("Clone was modified: %v", cloneData)
	}
}

// TestCPUBackend_SubBroadcasting tests broadcasting subtraction.
func TestCPUBackend_SubBroadcasting(t *testing.T) {
	backend := newTestBackend()

	// Test [2, 3] - [3] -> [2, 3]
	t.Run("BroadcastScalar", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		for i := range aData {
			aData[i] = float32(i + 10) // 10, 11, 12, 13, 14, 15
		}
		bData := b.AsFloat32()
		bData[0], bData[1], bData[2] = 1, 2, 3

		result := backend.Sub(a, b)

		// [10-1, 11-2, 12-3, 13-1, 14-2, 15-3]
		expected := []float32{9, 9, 9, 12, 12, 12}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Sub broadcast failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})

	// Test different dtypes
	t.Run("Float64", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		aData[0], aData[1], aData[2] = 10.5, 20.5, 30.5
		bData[0], bData[1], bData[2] = 0.5, 0.5, 0.5

		result := backend.Sub(a, b)

		expected := []float64{10.0, 20.0, 30.0}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 sub failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_MulBroadcasting tests broadcasting multiplication.
func TestCPUBackend_MulBroadcasting(t *testing.T) {
	backend := newTestBackend()

	// Test [2, 3] * [3] -> [2, 3]
	t.Run("BroadcastVector", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		for i := range aData {
			aData[i] = float32(i + 1) // 1, 2, 3, 4, 5, 6
		}
		bData := b.AsFloat32()
		bData[0], bData[1], bData[2] = 2, 3, 4

		result := backend.Mul(a, b)

		// [1*2, 2*3, 3*4, 4*2, 5*3, 6*4]
		expected := []float32{2, 6, 12, 8, 15, 24}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Mul broadcast failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})

	// Test int32
	t.Run("Int32", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		for i := range aData {
			aData[i] = int32(i + 1)
			bData[i] = 5
		}

		result := backend.Mul(a, b)

		expected := []int32{5, 10, 15, 20}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 mul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_DivBroadcasting tests broadcasting division.
func TestCPUBackend_DivBroadcasting(t *testing.T) {
	backend := newTestBackend()

	// Test [2, 3] / [3] -> [2, 3]
	t.Run("BroadcastVector", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		aData[0], aData[1], aData[2] = 10, 20, 30
		aData[3], aData[4], aData[5] = 40, 50, 60

		bData := b.AsFloat32()
		bData[0], bData[1], bData[2] = 2, 4, 5

		result := backend.Div(a, b)

		// [10/2, 20/4, 30/5, 40/2, 50/4, 60/5]
		expected := []float32{5, 5, 6, 20, 12.5, 12}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Div broadcast failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})

	// Test int64
	t.Run("Int64", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		aData[0], aData[1], aData[2] = 100, 200, 300
		bData[0], bData[1], bData[2] = 10, 20, 30

		result := backend.Div(a, b)

		expected := []int64{10, 10, 10}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 div failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_MatMulMultiDType tests MatMul with different data types.
func TestCPUBackend_MatMulMultiDType(t *testing.T) {
	backend := newTestBackend()

	// Test float64 MatMul
	t.Run("Float64_2x2", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		aData[0], aData[1], aData[2], aData[3] = 1.5, 2.5, 3.5, 4.5

		bData := b.AsFloat64()
		bData[0], bData[1], bData[2], bData[3] = 2, 0, 0, 2

		result := backend.MatMul(a, b)

		// [[1.5, 2.5],   [[2, 0],   [[3.0, 5.0],
		//  [3.5, 4.5]] *  [0, 2]] =  [7.0, 9.0]]
		expected := []float64{3.0, 5.0, 7.0, 9.0}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 matmul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	// Test int32 MatMul
	t.Run("Int32_3x3", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3, 3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3, 3}, tensor.Int32, tensor.CPU)

		// A = identity
		aData := a.AsInt32()
		aData[0], aData[1], aData[2] = 1, 0, 0
		aData[3], aData[4], aData[5] = 0, 1, 0
		aData[6], aData[7], aData[8] = 0, 0, 1

		// B = some matrix
		bData := b.AsInt32()
		for i := range bData {
			bData[i] = int32(i + 1)
		}

		result := backend.MatMul(a, b)

		// Identity * B = B
		expected := []int32{1, 2, 3, 4, 5, 6, 7, 8, 9}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 matmul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_TransposeMultiDType tests transpose with different dtypes.
func TestCPUBackend_TransposeMultiDType(t *testing.T) {
	backend := newTestBackend()

	// Test float64 transpose
	t.Run("Float64", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float64, tensor.CPU)
		aData := a.AsFloat64()
		for i := range aData {
			aData[i] = float64(i + 1)
		}

		result := backend.Transpose(a)

		expected := []float64{1, 4, 2, 5, 3, 6}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 transpose failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	// Test int32 transpose
	t.Run("Int32", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3, 2}, tensor.Int32, tensor.CPU)
		aData := a.AsInt32()
		for i := range aData {
			aData[i] = int32(i * 10)
		}

		result := backend.Transpose(a)

		// [[0, 10],     [[0, 20, 40],
		//  [20, 30],  ->  [10, 30, 50]]
		//  [40, 50]]
		expected := []int32{0, 20, 40, 10, 30, 50}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 transpose failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	// Test int64 transpose
	t.Run("Int64", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Int64, tensor.CPU)
		aData := a.AsInt64()
		aData[0], aData[1], aData[2], aData[3] = 1, 2, 3, 4

		result := backend.Transpose(a)

		expected := []int64{1, 3, 2, 4}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 transpose failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_ReshapeMultiDType tests reshape with different dtypes.
func TestCPUBackend_ReshapeMultiDType(t *testing.T) {
	backend := newTestBackend()

	// Test float64
	t.Run("Float64", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{6}, tensor.Float64, tensor.CPU)
		aData := a.AsFloat64()
		for i := range aData {
			aData[i] = float64(i + 1)
		}

		result := backend.Reshape(a, tensor.Shape{2, 3})

		if !result.Shape().Equal(tensor.Shape{2, 3}) {
			t.Errorf("Float64 reshape wrong shape: got %v", result.Shape())
		}

		expected := []float64{1, 2, 3, 4, 5, 6}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 reshape failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	// Test int32
	t.Run("Int32", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3, 2}, tensor.Int32, tensor.CPU)
		aData := a.AsInt32()
		for i := range aData {
			aData[i] = int32(i * 10)
		}

		result := backend.Reshape(a, tensor.Shape{2, 3})

		expected := []int32{0, 10, 20, 30, 40, 50}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 reshape failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_Float64VectorizedOps tests vectorized operations for float64.
//
//nolint:gocognit // Comprehensive test with multiple subtests for all float64 vectorized operations
func TestCPUBackend_Float64VectorizedOps(t *testing.T) {
	backend := newTestBackend()

	t.Run("Add_NonUnique", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		for i := range aData {
			aData[i] = float64(i + 1)
			bData[i] = float64((i + 1) * 10)
		}

		// Make 'a' non-unique by incrementing ref count
		_ = a.Clone()
		// Clone shares buffer

		result := backend.Add(a, b)

		expected := []float64{11, 22, 33, 44}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 vectorized add failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Sub_NonUnique", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		aData[0], aData[1], aData[2] = 10.0, 20.0, 30.0
		bData[0], bData[1], bData[2] = 1.0, 2.0, 3.0

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Sub(a, b)

		expected := []float64{9.0, 18.0, 27.0}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 vectorized sub failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Mul_NonUnique", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		aData[0], aData[1], aData[2] = 2.0, 3.0, 4.0
		bData[0], bData[1], bData[2] = 5.0, 6.0, 7.0

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Mul(a, b)

		expected := []float64{10.0, 18.0, 28.0}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 vectorized mul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Div_NonUnique", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		aData[0], aData[1], aData[2] = 20.0, 30.0, 40.0
		bData[0], bData[1], bData[2] = 2.0, 3.0, 4.0

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Div(a, b)

		expected := []float64{10.0, 10.0, 10.0}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 vectorized div failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_Float64InplaceOps tests inplace operations for float64.
func TestCPUBackend_Float64InplaceOps(t *testing.T) {
	backend := newTestBackend()

	t.Run("Mul_Inplace", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		aData[0], aData[1], aData[2] = 2.0, 3.0, 4.0
		bData[0], bData[1], bData[2] = 10.0, 20.0, 30.0

		result := backend.Mul(a, b)

		expected := []float64{20.0, 60.0, 120.0}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 inplace mul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Div_Inplace", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		aData[0], aData[1], aData[2] = 100.0, 200.0, 300.0
		bData[0], bData[1], bData[2] = 10.0, 20.0, 30.0

		result := backend.Div(a, b)

		expected := []float64{10.0, 10.0, 10.0}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 inplace div failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_Float64BroadcastOps tests broadcasting operations for float64.
//
//nolint:gocognit // Comprehensive test with multiple subtests for all float64 broadcast operations
func TestCPUBackend_Float64BroadcastOps(t *testing.T) {
	backend := newTestBackend()

	t.Run("Add_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		for i := range aData {
			aData[i] = float64(i)
		}
		bData[0], bData[1], bData[2] = 10.0, 20.0, 30.0

		result := backend.Add(a, b)

		expected := []float64{10, 21, 32, 13, 24, 35}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 broadcast add failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Sub_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		for i := range aData {
			aData[i] = float64(i + 10)
		}
		bData[0], bData[1], bData[2] = 1.0, 2.0, 3.0

		result := backend.Sub(a, b)

		expected := []float64{9, 9, 9, 12, 12, 12}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 broadcast sub failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Mul_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		for i := range aData {
			aData[i] = float64(i + 1)
		}
		bData[0], bData[1], bData[2] = 10.0, 10.0, 10.0

		result := backend.Mul(a, b)

		expected := []float64{10, 20, 30, 40, 50, 60}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 broadcast mul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Div_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float64, tensor.CPU)

		aData := a.AsFloat64()
		bData := b.AsFloat64()
		for i := range aData {
			aData[i] = float64((i + 1) * 10)
		}
		bData[0], bData[1], bData[2] = 2.0, 2.0, 2.0

		result := backend.Div(a, b)

		expected := []float64{5, 10, 15, 20, 25, 30}
		resultData := result.AsFloat64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Float64 broadcast div failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_Int32Operations tests all int32 operations.
//
//nolint:gocognit // Comprehensive test with multiple subtests for all int32 operations (inplace, vectorized, broadcast)
func TestCPUBackend_Int32Operations(t *testing.T) {
	backend := newTestBackend()

	t.Run("Add_Inplace", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		aData[0], aData[1], aData[2] = 1, 2, 3
		bData[0], bData[1], bData[2] = 10, 20, 30

		result := backend.Add(a, b)

		expected := []int32{11, 22, 33}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 add failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Sub_Inplace", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		aData[0], aData[1], aData[2] = 50, 40, 30
		bData[0], bData[1], bData[2] = 5, 4, 3

		result := backend.Sub(a, b)

		expected := []int32{45, 36, 27}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 sub failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Div_Inplace", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		aData[0], aData[1], aData[2] = 100, 200, 300
		bData[0], bData[1], bData[2] = 10, 20, 30

		result := backend.Div(a, b)

		expected := []int32{10, 10, 10}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 div failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Add_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		for i := range aData {
			aData[i] = int32(i)
			bData[i] = int32(i * 10)
		}

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Add(a, b)

		expected := []int32{0, 11, 22, 33}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 vectorized add failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Sub_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		aData[0], aData[1], aData[2] = 100, 200, 300
		bData[0], bData[1], bData[2] = 10, 20, 30

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Sub(a, b)

		expected := []int32{90, 180, 270}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 vectorized sub failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Mul_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		aData[0], aData[1], aData[2] = 2, 3, 4
		bData[0], bData[1], bData[2] = 5, 6, 7

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Mul(a, b)

		expected := []int32{10, 18, 28}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 vectorized mul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Div_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		aData[0], aData[1], aData[2] = 100, 200, 300
		bData[0], bData[1], bData[2] = 10, 20, 30

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Div(a, b)

		expected := []int32{10, 10, 10}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 vectorized div failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Add_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		for i := range aData {
			aData[i] = int32(i)
		}
		bData[0], bData[1], bData[2] = 10, 20, 30

		result := backend.Add(a, b)

		expected := []int32{10, 21, 32, 13, 24, 35}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 broadcast add failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Sub_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		for i := range aData {
			aData[i] = int32(i + 10)
		}
		bData[0], bData[1], bData[2] = 1, 2, 3

		result := backend.Sub(a, b)

		expected := []int32{9, 9, 9, 12, 12, 12}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 broadcast sub failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Mul_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		for i := range aData {
			aData[i] = int32(i + 1)
		}
		bData[0], bData[1], bData[2] = 10, 10, 10

		result := backend.Mul(a, b)

		expected := []int32{10, 20, 30, 40, 50, 60}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 broadcast mul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Div_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, tensor.CPU)

		aData := a.AsInt32()
		bData := b.AsInt32()
		for i := range aData {
			aData[i] = int32((i + 1) * 10)
		}
		bData[0], bData[1], bData[2] = 2, 2, 2

		result := backend.Div(a, b)

		expected := []int32{5, 10, 15, 20, 25, 30}
		resultData := result.AsInt32()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int32 broadcast div failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_Int64Operations tests all int64 operations.
//
//nolint:gocognit // Comprehensive test with multiple subtests for all int64 operations (inplace, vectorized, broadcast)
func TestCPUBackend_Int64Operations(t *testing.T) {
	backend := newTestBackend()

	t.Run("Add_Inplace", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		aData[0], aData[1], aData[2] = 1, 2, 3
		bData[0], bData[1], bData[2] = 10, 20, 30

		result := backend.Add(a, b)

		expected := []int64{11, 22, 33}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 add failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Sub_Inplace", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		aData[0], aData[1], aData[2] = 50, 40, 30
		bData[0], bData[1], bData[2] = 5, 4, 3

		result := backend.Sub(a, b)

		expected := []int64{45, 36, 27}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 sub failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Mul_Inplace", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		aData[0], aData[1], aData[2] = 2, 3, 4
		bData[0], bData[1], bData[2] = 5, 6, 7

		result := backend.Mul(a, b)

		expected := []int64{10, 18, 28}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 mul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Add_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		for i := range aData {
			aData[i] = int64(i)
			bData[i] = int64(i * 10)
		}

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Add(a, b)

		expected := []int64{0, 11, 22, 33}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 vectorized add failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Sub_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		aData[0], aData[1], aData[2] = 100, 200, 300
		bData[0], bData[1], bData[2] = 10, 20, 30

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Sub(a, b)

		expected := []int64{90, 180, 270}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 vectorized sub failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Mul_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		aData[0], aData[1], aData[2] = 2, 3, 4
		bData[0], bData[1], bData[2] = 5, 6, 7

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Mul(a, b)

		expected := []int64{10, 18, 28}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 vectorized mul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Div_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		aData[0], aData[1], aData[2] = 100, 200, 300
		bData[0], bData[1], bData[2] = 10, 20, 30

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Div(a, b)

		expected := []int64{10, 10, 10}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 vectorized div failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Add_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		for i := range aData {
			aData[i] = int64(i)
		}
		bData[0], bData[1], bData[2] = 10, 20, 30

		result := backend.Add(a, b)

		expected := []int64{10, 21, 32, 13, 24, 35}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 broadcast add failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Sub_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		for i := range aData {
			aData[i] = int64(i + 10)
		}
		bData[0], bData[1], bData[2] = 1, 2, 3

		result := backend.Sub(a, b)

		expected := []int64{9, 9, 9, 12, 12, 12}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 broadcast sub failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Mul_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		for i := range aData {
			aData[i] = int64(i + 1)
		}
		bData[0], bData[1], bData[2] = 10, 10, 10

		result := backend.Mul(a, b)

		expected := []int64{10, 20, 30, 40, 50, 60}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 broadcast mul failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})

	t.Run("Div_Broadcast", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Int64, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, tensor.CPU)

		aData := a.AsInt64()
		bData := b.AsInt64()
		for i := range aData {
			aData[i] = int64((i + 1) * 10)
		}
		bData[0], bData[1], bData[2] = 2, 2, 2

		result := backend.Div(a, b)

		expected := []int64{5, 10, 15, 20, 25, 30}
		resultData := result.AsInt64()
		for i, exp := range expected {
			if resultData[i] != exp {
				t.Errorf("Int64 broadcast div failed at %d: got %v, expected %v", i, resultData[i], exp)
			}
		}
	})
}

// TestCPUBackend_Float32VectorizedOps tests vectorized operations for float32.
func TestCPUBackend_Float32VectorizedOps(t *testing.T) {
	backend := newTestBackend()

	t.Run("Sub_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		bData := b.AsFloat32()
		aData[0], aData[1], aData[2] = 10.0, 20.0, 30.0
		bData[0], bData[1], bData[2] = 1.0, 2.0, 3.0

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Sub(a, b)

		expected := []float32{9.0, 18.0, 27.0}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Float32 vectorized sub failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})

	t.Run("Mul_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		bData := b.AsFloat32()
		aData[0], aData[1], aData[2] = 2.0, 3.0, 4.0
		bData[0], bData[1], bData[2] = 5.0, 6.0, 7.0

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Mul(a, b)

		expected := []float32{10.0, 18.0, 28.0}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Float32 vectorized mul failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})

	t.Run("Div_Vectorized", func(t *testing.T) {
		a, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)
		b, _ := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, tensor.CPU)

		aData := a.AsFloat32()
		bData := b.AsFloat32()
		aData[0], aData[1], aData[2] = 20.0, 30.0, 40.0
		bData[0], bData[1], bData[2] = 2.0, 3.0, 4.0

		_ = a.Clone()
		// Clone shares buffer

		result := backend.Div(a, b)

		expected := []float32{10.0, 10.0, 10.0}
		if !float32SliceEqual(result.AsFloat32(), expected) {
			t.Errorf("Float32 vectorized div failed: got %v, expected %v", result.AsFloat32(), expected)
		}
	})
}

// TestCPUBackend_Sigmoid verifies forward correctness of Sigmoid: σ(x) = 1/(1+exp(-x)).
func TestCPUBackend_Sigmoid(t *testing.T) {
	backend := newTestBackend()

	a, _ := tensor.NewRaw(tensor.Shape{5}, tensor.Float32, tensor.CPU)
	aData := a.AsFloat32()
	aData[0], aData[1], aData[2], aData[3], aData[4] = -2, -1, 0, 1, 2

	result := backend.Sigmoid(a)
	actual := result.AsFloat32()

	inputs := []float64{-2, -1, 0, 1, 2}
	const eps = 1e-5
	for i, x := range inputs {
		expected := float32(1.0 / (1.0 + math.Exp(-x)))
		diff := actual[i] - expected
		if diff < 0 {
			diff = -diff
		}
		if float64(diff) > eps {
			t.Errorf("Sigmoid[%d](%g) = %f, want %f", i, x, actual[i], expected)
		}
	}

	// Shape must be preserved.
	if !result.Shape().Equal(tensor.Shape{5}) {
		t.Errorf("Sigmoid shape = %v, want [5]", result.Shape())
	}
}

// TestCPUBackend_Tanh verifies forward correctness of Tanh.
func TestCPUBackend_Tanh(t *testing.T) {
	backend := newTestBackend()

	a, _ := tensor.NewRaw(tensor.Shape{5}, tensor.Float32, tensor.CPU)
	aData := a.AsFloat32()
	aData[0], aData[1], aData[2], aData[3], aData[4] = -2, -1, 0, 1, 2

	result := backend.Tanh(a)
	actual := result.AsFloat32()

	inputs := []float64{-2, -1, 0, 1, 2}
	const eps = 1e-5
	for i, x := range inputs {
		expected := float32(math.Tanh(x))
		diff := actual[i] - expected
		if diff < 0 {
			diff = -diff
		}
		if float64(diff) > eps {
			t.Errorf("Tanh[%d](%g) = %f, want %f", i, x, actual[i], expected)
		}
	}

	// Tanh(0) must be exactly 0.
	if actual[2] != 0 {
		t.Errorf("Tanh(0) = %f, want 0", actual[2])
	}
}

// TestCPUBackend_SiLU verifies forward correctness of SiLU: x·σ(x).
func TestCPUBackend_SiLU(t *testing.T) {
	backend := newTestBackend()

	a, _ := tensor.NewRaw(tensor.Shape{5}, tensor.Float32, tensor.CPU)
	aData := a.AsFloat32()
	aData[0], aData[1], aData[2], aData[3], aData[4] = -2, -1, 0, 1, 2

	result := backend.SiLU(a)
	actual := result.AsFloat32()

	inputs := []float64{-2, -1, 0, 1, 2}
	const eps = 1e-5
	for i, x := range inputs {
		sig := 1.0 / (1.0 + math.Exp(-x))
		expected := float32(x * sig)
		diff := actual[i] - expected
		if diff < 0 {
			diff = -diff
		}
		if float64(diff) > eps {
			t.Errorf("SiLU[%d](%g) = %f, want %f", i, x, actual[i], expected)
		}
	}

	// SiLU(0) must be exactly 0 (0 * 0.5 = 0).
	if actual[2] != 0 {
		t.Errorf("SiLU(0) = %f, want 0", actual[2])
	}
}

// TestSelfOperandAliasing verifies that binary ops with the same tensor as both
// operands (e.g. Mul(x, x)) do not mutate the input.
// Regression test for https://github.com/born-ml/born/issues/45
func TestSelfOperandAliasing(t *testing.T) {
	backend := newTestBackend()

	tests := []struct {
		name string
		op   func(a, b *tensor.RawTensor) *tensor.RawTensor
		want []float32
	}{
		{"Mul(x,x)", backend.Mul, []float32{2.25, 0.25, 0.25, 2.25}},
		{"Add(x,x)", backend.Add, []float32{-3, -1, 1, 3}},
		{"Sub(x,x)", backend.Sub, []float32{0, 0, 0, 0}},
		{"Div(x,x)", backend.Div, []float32{1, 1, 1, 1}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x, _ := tensor.FromSlice([]float32{-1.5, -0.5, 0.5, 1.5}, tensor.Shape{4}, backend)
			original := append([]float32(nil), x.Raw().AsFloat32()...)

			result := tc.op(x.Raw(), x.Raw())

			// Result must be correct
			if !float32SliceEqual(result.AsFloat32(), tc.want) {
				t.Errorf("result: got %v, want %v", result.AsFloat32(), tc.want)
			}

			// Input must NOT be mutated
			if !float32SliceEqual(x.Raw().AsFloat32(), original) {
				t.Errorf("input mutated: got %v, was %v", x.Raw().AsFloat32(), original)
			}
		})
	}
}
