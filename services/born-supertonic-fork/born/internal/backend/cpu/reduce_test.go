package cpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

func TestSumDim_1D(t *testing.T) {
	backend := New()

	// Test 1D tensor [4]
	x, _ := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	xData[0] = 1.0
	xData[1] = 2.0
	xData[2] = 3.0
	xData[3] = 4.0

	// Sum along dim 0 with keepDim=true -> [1]
	result := backend.SumDim(x, 0, true)
	if !result.Shape().Equal(tensor.Shape{1}) {
		t.Errorf("Expected shape [1], got %v", result.Shape())
	}
	expected := float32(10.0)
	if result.AsFloat32()[0] != expected {
		t.Errorf("Expected %v, got %v", expected, result.AsFloat32()[0])
	}

	// Sum along dim 0 with keepDim=false -> []
	result = backend.SumDim(x, 0, false)
	if len(result.Shape()) != 0 {
		t.Errorf("Expected shape [], got %v", result.Shape())
	}
	if result.AsFloat32()[0] != expected {
		t.Errorf("Expected %v, got %v", expected, result.AsFloat32()[0])
	}
}

func TestSumDim_2D_LastDim(t *testing.T) {
	backend := New()

	// Test 2D tensor [2, 3]
	x, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	// Row 0: [1, 2, 3]
	// Row 1: [4, 5, 6]
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Sum along dim -1 (last dim) with keepDim=true -> [2, 1]
	result := backend.SumDim(x, -1, true)
	if !result.Shape().Equal(tensor.Shape{2, 1}) {
		t.Errorf("Expected shape [2, 1], got %v", result.Shape())
	}
	resultData := result.AsFloat32()
	expectedRow0 := float32(6.0)  // 1+2+3
	expectedRow1 := float32(15.0) // 4+5+6
	if resultData[0] != expectedRow0 {
		t.Errorf("Row 0: expected %v, got %v", expectedRow0, resultData[0])
	}
	if resultData[1] != expectedRow1 {
		t.Errorf("Row 1: expected %v, got %v", expectedRow1, resultData[1])
	}

	// Sum along dim 1 with keepDim=false -> [2]
	result = backend.SumDim(x, 1, false)
	if !result.Shape().Equal(tensor.Shape{2}) {
		t.Errorf("Expected shape [2], got %v", result.Shape())
	}
	resultData = result.AsFloat32()
	if resultData[0] != expectedRow0 {
		t.Errorf("Row 0: expected %v, got %v", expectedRow0, resultData[0])
	}
	if resultData[1] != expectedRow1 {
		t.Errorf("Row 1: expected %v, got %v", expectedRow1, resultData[1])
	}
}

func TestSumDim_2D_FirstDim(t *testing.T) {
	backend := New()

	// Test 2D tensor [2, 3]
	x, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	// Row 0: [1, 2, 3]
	// Row 1: [4, 5, 6]
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Sum along dim 0 with keepDim=true -> [1, 3]
	result := backend.SumDim(x, 0, true)
	if !result.Shape().Equal(tensor.Shape{1, 3}) {
		t.Errorf("Expected shape [1, 3], got %v", result.Shape())
	}
	resultData := result.AsFloat32()
	expected := []float32{5.0, 7.0, 9.0} // [1+4, 2+5, 3+6]
	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("Index %d: expected %v, got %v", i, exp, resultData[i])
		}
	}

	// Sum along dim 0 with keepDim=false -> [3]
	result = backend.SumDim(x, 0, false)
	if !result.Shape().Equal(tensor.Shape{3}) {
		t.Errorf("Expected shape [3], got %v", result.Shape())
	}
	resultData = result.AsFloat32()
	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("Index %d: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

func TestSumDim_3D(t *testing.T) {
	backend := New()

	// Test 3D tensor [2, 3, 4]
	x, _ := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Sum along dim -1 (last dim) with keepDim=true -> [2, 3, 1]
	result := backend.SumDim(x, -1, true)
	if !result.Shape().Equal(tensor.Shape{2, 3, 1}) {
		t.Errorf("Expected shape [2, 3, 1], got %v", result.Shape())
	}

	// Sum along middle dim with keepDim=false -> [2, 4]
	result = backend.SumDim(x, 1, false)
	if !result.Shape().Equal(tensor.Shape{2, 4}) {
		t.Errorf("Expected shape [2, 4], got %v", result.Shape())
	}
}

func TestMeanDim_2D(t *testing.T) {
	backend := New()

	// Test 2D tensor [2, 4]
	x, _ := tensor.NewRaw(tensor.Shape{2, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	// Row 0: [1, 2, 3, 4]
	// Row 1: [5, 6, 7, 8]
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Mean along dim -1 with keepDim=true -> [2, 1]
	result := backend.MeanDim(x, -1, true)
	if !result.Shape().Equal(tensor.Shape{2, 1}) {
		t.Errorf("Expected shape [2, 1], got %v", result.Shape())
	}
	resultData := result.AsFloat32()
	expectedRow0 := float32(2.5) // (1+2+3+4)/4
	expectedRow1 := float32(6.5) // (5+6+7+8)/4
	if resultData[0] != expectedRow0 {
		t.Errorf("Row 0: expected %v, got %v", expectedRow0, resultData[0])
	}
	if resultData[1] != expectedRow1 {
		t.Errorf("Row 1: expected %v, got %v", expectedRow1, resultData[1])
	}

	// Mean along dim 0 with keepDim=false -> [4]
	result = backend.MeanDim(x, 0, false)
	if !result.Shape().Equal(tensor.Shape{4}) {
		t.Errorf("Expected shape [4], got %v", result.Shape())
	}
	resultData = result.AsFloat32()
	expected := []float32{3.0, 4.0, 5.0, 6.0} // [(1+5)/2, (2+6)/2, (3+7)/2, (4+8)/2]
	for i, exp := range expected {
		if resultData[i] != exp {
			t.Errorf("Index %d: expected %v, got %v", i, exp, resultData[i])
		}
	}
}

func TestMeanDim_RMSNormPattern(t *testing.T) {
	backend := New()

	// RMSNorm pattern: x^2 then mean along last dim
	// Test tensor [2, 3, 4]
	x, _ := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = float32(i + 1)
	}

	// Square the tensor (element-wise)
	xSquared, _ := tensor.NewRaw(x.Shape(), x.DType(), backend.Device())
	xSquaredData := xSquared.AsFloat32()
	for i, v := range xData {
		xSquaredData[i] = v * v
	}

	// Mean along last dim with keepDim=true -> [2, 3, 1]
	variance := backend.MeanDim(xSquared, -1, true)
	if !variance.Shape().Equal(tensor.Shape{2, 3, 1}) {
		t.Errorf("Expected shape [2, 3, 1], got %v", variance.Shape())
	}

	// Check that variance has reasonable values
	varianceData := variance.AsFloat32()
	for i, v := range varianceData {
		if v <= 0 {
			t.Errorf("Variance at %d should be positive, got %v", i, v)
		}
	}
}

func TestSumDim_NegativeDim(t *testing.T) {
	backend := New()

	// Test negative dimension indexing
	x, _ := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, backend.Device())
	xData := x.AsFloat32()
	for i := range xData {
		xData[i] = 1.0
	}

	// dim=-1 should be equivalent to dim=2
	result1 := backend.SumDim(x, -1, true)
	result2 := backend.SumDim(x, 2, true)

	if !result1.Shape().Equal(result2.Shape()) {
		t.Errorf("Shapes don't match: dim=-1 gave %v, dim=2 gave %v", result1.Shape(), result2.Shape())
	}

	// dim=-2 should be equivalent to dim=1
	result3 := backend.SumDim(x, -2, false)
	result4 := backend.SumDim(x, 1, false)

	if !result3.Shape().Equal(result4.Shape()) {
		t.Errorf("Shapes don't match: dim=-2 gave %v, dim=1 gave %v", result3.Shape(), result4.Shape())
	}
}

func TestSumDim_Float64(t *testing.T) {
	backend := New()

	// Test with float64
	x, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float64, backend.Device())
	xData := x.AsFloat64()
	for i := range xData {
		xData[i] = float64(i + 1)
	}

	result := backend.SumDim(x, -1, true)
	if !result.Shape().Equal(tensor.Shape{2, 1}) {
		t.Errorf("Expected shape [2, 1], got %v", result.Shape())
	}

	resultData := result.AsFloat64()
	expectedRow0 := 6.0  // 1+2+3
	expectedRow1 := 15.0 // 4+5+6
	if resultData[0] != expectedRow0 {
		t.Errorf("Row 0: expected %v, got %v", expectedRow0, resultData[0])
	}
	if resultData[1] != expectedRow1 {
		t.Errorf("Row 1: expected %v, got %v", expectedRow1, resultData[1])
	}
}

func TestMeanDim_Float64(t *testing.T) {
	backend := New()

	// Test with float64
	x, _ := tensor.NewRaw(tensor.Shape{2, 4}, tensor.Float64, backend.Device())
	xData := x.AsFloat64()
	for i := range xData {
		xData[i] = float64(i + 1)
	}

	result := backend.MeanDim(x, -1, true)
	resultData := result.AsFloat64()

	expectedRow0 := 2.5 // (1+2+3+4)/4
	expectedRow1 := 6.5 // (5+6+7+8)/4

	if resultData[0] != expectedRow0 {
		t.Errorf("Row 0: expected %v, got %v", expectedRow0, resultData[0])
	}
	if resultData[1] != expectedRow1 {
		t.Errorf("Row 1: expected %v, got %v", expectedRow1, resultData[1])
	}
}
