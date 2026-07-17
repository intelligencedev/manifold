package cpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestGather1D tests gather on 1D tensors.
func TestGather1D(t *testing.T) {
	backend := New()

	// Input: [10, 20, 30, 40]
	input, err := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input tensor: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32((i + 1) * 10)
	}

	// Index: [2, 0, 3] (gather indices 2, 0, 3)
	index, err := tensor.NewRaw(tensor.Shape{3}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index tensor: %v", err)
	}
	indexData := index.AsInt32()
	indexData[0] = 2
	indexData[1] = 0
	indexData[2] = 3

	// Gather
	result := backend.Gather(input, 0, index)

	// Expected: [30, 10, 40]
	expected := []float32{30, 10, 40}
	resultData := result.AsFloat32()
	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("Gather1D result[%d] = %f, expected %f", i, resultData[i], exp)
		}
	}
}

// TestGather2D tests gather on 2D tensors.
func TestGather2D(t *testing.T) {
	backend := New()

	// Input: [[10, 20, 30],
	//         [40, 50, 60]]
	input, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input tensor: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32((i + 1) * 10)
	}

	// Index: [[2, 0],
	//         [1, 2]] (gather along dim 1)
	index, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index tensor: %v", err)
	}
	indexData := index.AsInt32()
	indexData[0] = 2 // row 0, col 0: get input[0,2] = 30
	indexData[1] = 0 // row 0, col 1: get input[0,0] = 10
	indexData[2] = 1 // row 1, col 0: get input[1,1] = 50
	indexData[3] = 2 // row 1, col 1: get input[1,2] = 60

	// Gather along dim 1 (columns)
	result := backend.Gather(input, 1, index)

	// Expected: [[30, 10],
	//            [50, 60]]
	expected := []float32{30, 10, 50, 60}
	resultData := result.AsFloat32()
	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("Gather2D result[%d] = %f, expected %f", i, resultData[i], exp)
		}
	}
}

// TestGather3D tests gather on 3D tensors.
func TestGather3D(t *testing.T) {
	backend := New()

	// Input: [2, 3, 4] tensor with sequential values
	input, err := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input tensor: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i)
	}

	// Index: [2, 3, 2] (gather along dim 2)
	index, err := tensor.NewRaw(tensor.Shape{2, 3, 2}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index tensor: %v", err)
	}
	indexData := index.AsInt32()
	for i := range indexData {
		indexData[i] = int32(i % 4) // indices in range [0, 3]
	}

	// Gather along dim 2
	result := backend.Gather(input, 2, index)

	// Verify shape
	if !result.Shape().Equal(tensor.Shape{2, 3, 2}) {
		t.Errorf("Gather3D result shape = %v, expected [2, 3, 2]", result.Shape())
	}

	// Verify some values
	resultData := result.AsFloat32()
	// input[0,0,:] = [0,1,2,3], gather indices [0,1] -> [0,1]
	if resultData[0] != 0 || resultData[1] != 1 {
		t.Errorf("Gather3D result[0:2] = [%f, %f], expected [0, 1]",
			resultData[0], resultData[1])
	}
}

// TestGatherNegativeDim tests gather with negative dimension.
func TestGatherNegativeDim(t *testing.T) {
	backend := New()

	input, err := tensor.NewRaw(tensor.Shape{3, 4}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input tensor: %v", err)
	}
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i)
	}

	index, err := tensor.NewRaw(tensor.Shape{3, 2}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index tensor: %v", err)
	}
	indexData := index.AsInt32()
	for i := range indexData {
		indexData[i] = int32(i % 4)
	}

	// Gather with dim=-1 (last dimension, same as dim=1)
	result := backend.Gather(input, -1, index)

	if !result.Shape().Equal(tensor.Shape{3, 2}) {
		t.Errorf("Gather negative dim result shape = %v, expected [3, 2]", result.Shape())
	}
}

// TestGatherOutOfBounds tests gather panics on out-of-bounds index.
func TestGatherOutOfBounds(t *testing.T) {
	backend := New()

	input, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create input tensor: %v", err)
	}

	index, err := tensor.NewRaw(tensor.Shape{2}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create index tensor: %v", err)
	}
	indexData := index.AsInt32()
	indexData[0] = 5 // Out of bounds!
	indexData[1] = 0

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Gather with out-of-bounds index should panic")
		}
	}()

	backend.Gather(input, 0, index)
}

// TestWhereSimple tests where with simple boolean condition.
func TestWhereSimple(t *testing.T) {
	backend := New()

	// Condition: [true, false, true]
	condition, err := tensor.NewRaw(tensor.Shape{3}, tensor.Bool, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create condition tensor: %v", err)
	}
	condData := condition.AsBool()
	condData[0] = true  // true
	condData[1] = false // false
	condData[2] = true  // true

	// X: [10, 20, 30]
	x, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create x tensor: %v", err)
	}
	xData := x.AsFloat32()
	xData[0] = 10
	xData[1] = 20
	xData[2] = 30

	// Y: [100, 200, 300]
	y, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create y tensor: %v", err)
	}
	yData := y.AsFloat32()
	yData[0] = 100
	yData[1] = 200
	yData[2] = 300

	// Where
	result := backend.Where(condition, x, y)

	// Expected: [10, 200, 30] (true->x, false->y)
	expected := []float32{10, 200, 30}
	resultData := result.AsFloat32()
	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("Where result[%d] = %f, expected %f", i, resultData[i], exp)
		}
	}
}

// TestWhere2D tests where with 2D tensors.
func TestWhere2D(t *testing.T) {
	backend := New()

	// Condition: [[true, false],
	//             [false, true]]
	condition, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Bool, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create condition tensor: %v", err)
	}
	condData := condition.AsBool()
	condData[0] = true  // true
	condData[1] = false // false
	condData[2] = false // false
	condData[3] = true  // true

	// X: [[1, 2],
	//     [3, 4]]
	x, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create x tensor: %v", err)
	}
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Y: [[10, 20],
	//     [30, 40]]
	y, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create y tensor: %v", err)
	}
	yData := y.AsFloat32()
	for i := range yData {
		yData[i] = float32((i + 1) * 10)
	}

	// Where
	result := backend.Where(condition, x, y)

	// Expected: [[1, 20],
	//            [30, 4]]
	expected := []float32{1, 20, 30, 4}
	resultData := result.AsFloat32()
	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("Where2D result[%d] = %f, expected %f", i, resultData[i], exp)
		}
	}
}

// TestWhereBroadcast tests where with broadcasting.
func TestWhereBroadcast(t *testing.T) {
	backend := New()

	// Condition: [true, false] (shape [2])
	condition, err := tensor.NewRaw(tensor.Shape{2}, tensor.Bool, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create condition tensor: %v", err)
	}
	condData := condition.AsBool()
	condData[0] = true  // true
	condData[1] = false // false

	// X: [[1, 2],
	//     [3, 4]] (shape [2, 2])
	x, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create x tensor: %v", err)
	}
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Y: 100 (scalar, shape [1])
	y, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create y tensor: %v", err)
	}
	yData := y.AsFloat32()
	yData[0] = 100

	// Where (with broadcasting)
	result := backend.Where(condition, x, y)

	// Expected: [[1, 100],
	//            [3, 100]]
	// condition broadcasts to [[true, false], [true, false]]
	// y broadcasts to [[100, 100], [100, 100]]
	expected := []float32{1, 100, 3, 100}
	resultData := result.AsFloat32()
	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("WhereBroadcast result[%d] = %f, expected %f", i, resultData[i], exp)
		}
	}
}

// TestWhereAllTrue tests where with all true condition.
func TestWhereAllTrue(t *testing.T) {
	backend := New()

	condition, err := tensor.NewRaw(tensor.Shape{3}, tensor.Bool, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create condition tensor: %v", err)
	}
	condData := condition.AsBool()
	for i := range condData {
		condData[i] = true // all true
	}

	x, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create x tensor: %v", err)
	}
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	y, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create y tensor: %v", err)
	}
	yData := y.AsFloat32()
	for i := range yData {
		yData[i] = float32((i + 1) * 100)
	}

	result := backend.Where(condition, x, y)

	// Expected: all from x
	resultData := result.AsFloat32()
	for i := range xData {
		if resultData[i] != xData[i] {
			t.Errorf("WhereAllTrue result[%d] = %f, expected %f", i, resultData[i], xData[i])
		}
	}
}

// TestWhereAllFalse tests where with all false condition.
func TestWhereAllFalse(t *testing.T) {
	backend := New()

	condition, err := tensor.NewRaw(tensor.Shape{3}, tensor.Bool, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create condition tensor: %v", err)
	}
	// All zeros (false) by default

	x, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create x tensor: %v", err)
	}
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	y, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create y tensor: %v", err)
	}
	yData := y.AsFloat32()
	for i := range yData {
		yData[i] = float32((i + 1) * 100)
	}

	result := backend.Where(condition, x, y)

	// Expected: all from y
	resultData := result.AsFloat32()
	for i := range yData {
		if resultData[i] != yData[i] {
			t.Errorf("WhereAllFalse result[%d] = %f, expected %f", i, resultData[i], yData[i])
		}
	}
}

// TestWhereUInt8Condition tests where with uint8 condition (non-zero = true).
func TestWhereUInt8Condition(t *testing.T) {
	backend := New()

	// Condition: [1, 0, 5] (uint8: non-zero = true)
	condition, err := tensor.NewRaw(tensor.Shape{3}, tensor.Uint8, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create condition tensor: %v", err)
	}
	condData := condition.AsUint8()
	condData[0] = 1 // true
	condData[1] = 0 // false
	condData[2] = 5 // true (non-zero)

	x, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create x tensor: %v", err)
	}
	xData := x.AsFloat32()
	xData[0] = 10
	xData[1] = 20
	xData[2] = 30

	y, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create y tensor: %v", err)
	}
	yData := y.AsFloat32()
	yData[0] = 100
	yData[1] = 200
	yData[2] = 300

	result := backend.Where(condition, x, y)

	// Expected: [10, 200, 30]
	expected := []float32{10, 200, 30}
	resultData := result.AsFloat32()
	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("WhereUInt8 result[%d] = %f, expected %f", i, resultData[i], exp)
		}
	}
}
