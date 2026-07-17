package tensor

import (
	"math"
	"testing"
)

// Softmax Tests

func TestTensorSoftmax(t *testing.T) {
	t.Skip("Softmax requires full backend implementation")
	backend := NewMockBackend()
	// [[1, 2, 3],
	//  [4, 5, 6]]
	tensor, _ := FromSlice([]float32{1, 2, 3, 4, 5, 6}, Shape{2, 3}, backend)

	// Softmax along dim 1 (across columns)
	result := tensor.Softmax(1)

	assertEqualShape(t, Shape{2, 3}, result.Shape(), "Softmax shape")

	// Check that each row sums to 1
	for i := 0; i < 2; i++ {
		sum := float32(0)
		for j := 0; j < 3; j++ {
			val := result.At(i, j)
			if val < 0 || val > 1 {
				t.Errorf("Softmax[%d,%d] = %v, should be in [0, 1]", i, j, val)
			}
			sum += val
		}
		if math.Abs(float64(sum-1)) > 1e-5 {
			t.Errorf("Softmax row %d sum = %v, want 1", i, sum)
		}
	}

	// Check that values are in increasing order in each row (since input is increasing)
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			if result.At(i, j) >= result.At(i, j+1) {
				t.Errorf("Softmax[%d,%d] = %v should be < Softmax[%d,%d] = %v",
					i, j, result.At(i, j), i, j+1, result.At(i, j+1))
			}
		}
	}
}

func TestTensorSoftmaxDim0(t *testing.T) {
	t.Skip("Softmax requires full backend implementation")
	t.Skip("Softmax requires full backend implementation")
	backend := NewMockBackend()
	// [[1, 2],
	//  [3, 4]]
	tensor, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)

	// Softmax along dim 0 (across rows)
	result := tensor.Softmax(0)

	assertEqualShape(t, Shape{2, 2}, result.Shape(), "Softmax dim 0 shape")

	// Check that each column sums to 1
	for j := 0; j < 2; j++ {
		sum := float32(0)
		for i := 0; i < 2; i++ {
			val := result.At(i, j)
			sum += val
		}
		if math.Abs(float64(sum-1)) > 1e-5 {
			t.Errorf("Softmax column %d sum = %v, want 1", j, sum)
		}
	}
}

func TestTensorSoftmax1D(t *testing.T) {
	t.Skip("Softmax requires full backend implementation")
	t.Skip("Softmax requires full backend implementation")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1, 2, 3, 4, 5}, Shape{5}, backend)

	result := tensor.Softmax(0)

	assertEqualShape(t, Shape{5}, result.Shape(), "Softmax 1D shape")

	// Check sum equals 1
	sum := float32(0)
	data := result.Data()
	for _, v := range data {
		sum += v
	}

	if math.Abs(float64(sum-1)) > 1e-5 {
		t.Errorf("Softmax 1D sum = %v, want 1", sum)
	}

	// Check values are monotonically increasing (since input is increasing)
	for i := 0; i < 4; i++ {
		if data[i] >= data[i+1] {
			t.Errorf("Softmax[%d] = %v should be < Softmax[%d] = %v",
				i, data[i], i+1, data[i+1])
		}
	}
}

// Where Tests

func TestTensorWhereTernary(t *testing.T) {
	t.Skip("Where requires full backend implementation")
	backend := NewMockBackend()

	// condition: [true, false, true, false]
	condition, _ := FromSlice([]bool{true, false, true, false}, Shape{4}, backend)

	// x: [1, 2, 3, 4]
	x, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{4}, backend)

	// y: [10, 20, 30, 40]
	y, _ := FromSlice([]float32{10, 20, 30, 40}, Shape{4}, backend)

	// result should be: [1, 20, 3, 40]
	result := Where(condition, x, y)

	expected := []float32{1, 20, 3, 40}
	got := result.Data()
	for i, exp := range expected {
		assertEqualFloat32(t, exp, got[i], "Where")
	}
}

func TestTensorWhere2D(t *testing.T) {
	t.Skip("Where requires full backend implementation")
	backend := NewMockBackend()

	// condition: [[true, false],
	//             [false, true]]
	condition, _ := FromSlice([]bool{true, false, false, true}, Shape{2, 2}, backend)

	// x: [[1, 2],
	//     [3, 4]]
	x, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)

	// y: [[10, 20],
	//     [30, 40]]
	y, _ := FromSlice([]float32{10, 20, 30, 40}, Shape{2, 2}, backend)

	// result should be: [[1, 20],
	//                    [30, 4]]
	result := Where(condition, x, y)

	assertEqualFloat32(t, 1, result.At(0, 0), "Where[0,0]")
	assertEqualFloat32(t, 20, result.At(0, 1), "Where[0,1]")
	assertEqualFloat32(t, 30, result.At(1, 0), "Where[1,0]")
	assertEqualFloat32(t, 4, result.At(1, 1), "Where[1,1]")
}

func TestTensorWhereBroadcast(t *testing.T) {
	t.Skip("Where requires full backend implementation")
	backend := NewMockBackend()

	// condition: [true, false] (shape: 2)
	condition, _ := FromSlice([]bool{true, false}, Shape{2}, backend)

	// x: [[1, 2]] (shape: 1, 2)
	x, _ := FromSlice([]float32{1, 2}, Shape{1, 2}, backend)

	// y: [[10],
	//     [20]] (shape: 2, 1)
	y, _ := FromSlice([]float32{10, 20}, Shape{2, 1}, backend)

	// With broadcasting:
	// condition broadcasts to (2, 2): [[true, false], [true, false]]
	// x broadcasts to (2, 2): [[1, 2], [1, 2]]
	// y broadcasts to (2, 2): [[10, 10], [20, 20]]
	// result should be: [[1, 10], [1, 20]]
	result := Where(condition, x, y)

	assertEqualShape(t, Shape{2, 2}, result.Shape(), "Where broadcast shape")

	assertEqualFloat32(t, 1, result.At(0, 0), "Where broadcast [0,0]")
	assertEqualFloat32(t, 10, result.At(0, 1), "Where broadcast [0,1]")
	assertEqualFloat32(t, 1, result.At(1, 0), "Where broadcast [1,0]")
	assertEqualFloat32(t, 20, result.At(1, 1), "Where broadcast [1,1]")
}

func TestTensorWhereAllTrue(t *testing.T) {
	t.Skip("Where requires full backend implementation")
	backend := NewMockBackend()

	condition, _ := FromSlice([]bool{true, true, true}, Shape{3}, backend)
	x, _ := FromSlice([]float32{1, 2, 3}, Shape{3}, backend)
	y, _ := FromSlice([]float32{10, 20, 30}, Shape{3}, backend)

	result := Where(condition, x, y)

	// Should return all from x
	expected := []float32{1, 2, 3}
	got := result.Data()
	for i, exp := range expected {
		assertEqualFloat32(t, exp, got[i], "Where all true")
	}
}

func TestTensorWhereAllFalse(t *testing.T) {
	t.Skip("Where requires full backend implementation")
	backend := NewMockBackend()

	condition, _ := FromSlice([]bool{false, false, false}, Shape{3}, backend)
	x, _ := FromSlice([]float32{1, 2, 3}, Shape{3}, backend)
	y, _ := FromSlice([]float32{10, 20, 30}, Shape{3}, backend)

	result := Where(condition, x, y)

	// Should return all from y
	expected := []float32{10, 20, 30}
	got := result.Data()
	for i, exp := range expected {
		assertEqualFloat32(t, exp, got[i], "Where all false")
	}
}

func TestTensorWhereInt(t *testing.T) {
	t.Skip("Where requires full backend implementation")
	backend := NewMockBackend()

	condition, _ := FromSlice([]bool{true, false, true}, Shape{3}, backend)
	x, _ := FromSlice([]int32{1, 2, 3}, Shape{3}, backend)
	y, _ := FromSlice([]int32{10, 20, 30}, Shape{3}, backend)

	result := Where(condition, x, y)

	expected := []int32{1, 20, 3}
	got := result.Data()
	for i, exp := range expected {
		if got[i] != exp {
			t.Errorf("Where int[%d] = %v, want %v", i, got[i], exp)
		}
	}
}

// Test manipulation.go Where function (non-method version)

func TestWhereFunction(t *testing.T) {
	t.Skip("Where requires full backend implementation")
	backend := NewMockBackend()

	condition, _ := FromSlice([]bool{true, false, true, false}, Shape{4}, backend)
	x, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{4}, backend)
	y, _ := FromSlice([]float32{10, 20, 30, 40}, Shape{4}, backend)

	result := Where(condition, x, y)

	expected := []float32{1, 20, 3, 40}
	got := result.Data()
	for i, exp := range expected {
		assertEqualFloat32(t, exp, got[i], "Where function")
	}
}
