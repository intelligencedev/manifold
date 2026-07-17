package tensor

import (
	"fmt"
	"math"
	"testing"
)

// Division Tests

func TestTensorDiv(t *testing.T) {
	t.Skip("Div not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{10, 20, 30, 40}, Shape{2, 2}, backend)
	b, _ := FromSlice([]float32{2, 4, 5, 8}, Shape{2, 2}, backend)

	c := a.Div(b)

	expected := []float32{5, 5, 6, 5}
	got := c.Data()

	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("Div[%d]", i))
	}
}

// BatchMatMul Tests

func TestTensorBatchMatMul(t *testing.T) {
	t.Skip("BatchMatMul not implemented in MockBackend")
	backend := NewMockBackend()
	// Batch of 2 matrices: (2, 2, 2) @ (2, 2, 2) → (2, 2, 2)
	a, _ := FromSlice([]float32{1, 2, 3, 4, 5, 6, 7, 8}, Shape{2, 2, 2}, backend)
	b, _ := FromSlice([]float32{1, 0, 0, 1, 2, 0, 0, 2}, Shape{2, 2, 2}, backend)

	c := a.BatchMatMul(b)

	assertEqualShape(t, Shape{2, 2, 2}, c.Shape(), "BatchMatMul shape")

	// First batch: [[1,2],[3,4]] @ [[1,0],[0,1]] = [[1,2],[3,4]]
	assertEqualFloat32(t, 1, c.At(0, 0, 0), "BatchMatMul[0,0,0]")
	assertEqualFloat32(t, 2, c.At(0, 0, 1), "BatchMatMul[0,0,1]")
	assertEqualFloat32(t, 3, c.At(0, 1, 0), "BatchMatMul[0,1,0]")
	assertEqualFloat32(t, 4, c.At(0, 1, 1), "BatchMatMul[0,1,1]")
}

// Reduction Tests

func TestTensorSumDim(t *testing.T) {
	t.Skip("SumDim not implemented in MockBackend")
	t.Skip("Sum not implemented in MockBackend")
	backend := NewMockBackend()
	// [[1, 2, 3],
	//  [4, 5, 6]]
	tensor, _ := FromSlice([]float32{1, 2, 3, 4, 5, 6}, Shape{2, 3}, backend)

	// Sum along dim 0 (reduce rows)
	sum0 := tensor.SumDim(0, false)
	assertEqualShape(t, Shape{3}, sum0.Shape(), "SumDim(0) shape")
	expected0 := []float32{5, 7, 9} // [1+4, 2+5, 3+6]
	for i, exp := range expected0 {
		assertEqualFloat32(t, exp, sum0.At(i), fmt.Sprintf("SumDim(0)[%d]", i))
	}

	// Sum along dim 1 (reduce columns)
	sum1 := tensor.SumDim(1, false)
	assertEqualShape(t, Shape{2}, sum1.Shape(), "SumDim(1) shape")
	expected1 := []float32{6, 15} // [1+2+3, 4+5+6]
	for i, exp := range expected1 {
		assertEqualFloat32(t, exp, sum1.At(i), fmt.Sprintf("SumDim(1)[%d]", i))
	}

	// Sum with keepdim
	sum0Keep := tensor.SumDim(0, true)
	assertEqualShape(t, Shape{1, 3}, sum0Keep.Shape(), "SumDim(0, keepdim) shape")
}

func TestTensorMeanDim(t *testing.T) {
	t.Skip("MeanDim not implemented in MockBackend")
	backend := NewMockBackend()
	// [[2, 4, 6],
	//  [8, 10, 12]]
	tensor, _ := FromSlice([]float32{2, 4, 6, 8, 10, 12}, Shape{2, 3}, backend)

	// Mean along dim 0
	mean0 := tensor.MeanDim(0, false)
	assertEqualShape(t, Shape{3}, mean0.Shape(), "MeanDim(0) shape")
	expected0 := []float32{5, 7, 9} // [(2+8)/2, (4+10)/2, (6+12)/2]
	for i, exp := range expected0 {
		assertEqualFloat32(t, exp, mean0.At(i), fmt.Sprintf("MeanDim(0)[%d]", i))
	}

	// Mean along dim 1
	mean1 := tensor.MeanDim(1, false)
	assertEqualShape(t, Shape{2}, mean1.Shape(), "MeanDim(1) shape")
	expected1 := []float32{4, 10} // [(2+4+6)/3, (8+10+12)/3]
	for i, exp := range expected1 {
		assertEqualFloat32(t, exp, mean1.At(i), fmt.Sprintf("MeanDim(1)[%d]", i))
	}
}

// Scalar Operations Tests

func TestTensorMulScalar(t *testing.T) {
	t.Skip("MulScalar not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)

	result := tensor.MulScalar(2.5)

	expected := []float32{2.5, 5, 7.5, 10}
	got := result.Data()
	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("MulScalar[%d]", i))
	}
}

func TestTensorAddScalar(t *testing.T) {
	t.Skip("AddScalar not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1, 2, 3, 4}, Shape{2, 2}, backend)

	result := tensor.AddScalar(10)

	expected := []float32{11, 12, 13, 14}
	got := result.Data()
	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("AddScalar[%d]", i))
	}
}

func TestTensorSubScalar(t *testing.T) {
	t.Skip("SubScalar not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{10, 20, 30, 40}, Shape{2, 2}, backend)

	result := tensor.SubScalar(5)

	expected := []float32{5, 15, 25, 35}
	got := result.Data()
	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("SubScalar[%d]", i))
	}
}

func TestTensorDivScalar(t *testing.T) {
	t.Skip("Div not implemented in MockBackend")
	t.Skip("DivScalar not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{10, 20, 30, 40}, Shape{2, 2}, backend)

	result := tensor.DivScalar(10)

	expected := []float32{1, 2, 3, 4}
	got := result.Data()
	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("DivScalar[%d]", i))
	}
}

// Mathematical Functions Tests

func TestTensorExp(t *testing.T) {
	t.Skip("Exp not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{0, 1, 2}, Shape{3}, backend)

	result := tensor.Exp()

	expected := []float32{1, 2.718281828, 7.389056099}
	got := result.Data()
	for i := range expected {
		if math.Abs(float64(got[i]-expected[i])) > 1e-5 {
			t.Errorf("Exp[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorLog(t *testing.T) {
	t.Skip("Log not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1, 2.718281828, 7.389056099}, Shape{3}, backend)

	result := tensor.Log()

	expected := []float32{0, 1, 2}
	got := result.Data()
	for i := range expected {
		if math.Abs(float64(got[i]-expected[i])) > 1e-5 {
			t.Errorf("Log[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorSqrt(t *testing.T) {
	t.Skip("Sqrt not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1, 4, 9, 16}, Shape{4}, backend)

	result := tensor.Sqrt()

	expected := []float32{1, 2, 3, 4}
	got := result.Data()
	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("Sqrt[%d]", i))
	}
}

func TestTensorRsqrt(t *testing.T) {
	t.Skip("Rsqrt not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1, 4, 9, 16}, Shape{4}, backend)

	result := tensor.Rsqrt()

	expected := []float32{1, 0.5, 0.333333, 0.25}
	got := result.Data()
	for i := range expected {
		if math.Abs(float64(got[i]-expected[i])) > 1e-5 {
			t.Errorf("Rsqrt[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorCos(t *testing.T) {
	t.Skip("Cos not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{0, math.Pi / 2, math.Pi}, Shape{3}, backend)

	result := tensor.Cos()

	expected := []float32{1, 0, -1}
	got := result.Data()
	for i := range expected {
		if math.Abs(float64(got[i]-expected[i])) > 1e-5 {
			t.Errorf("Cos[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorSin(t *testing.T) {
	t.Skip("Sin not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{0, math.Pi / 2, math.Pi}, Shape{3}, backend)

	result := tensor.Sin()

	expected := []float32{0, 1, 0}
	got := result.Data()
	for i := range expected {
		if math.Abs(float64(got[i]-expected[i])) > 1e-5 {
			t.Errorf("Sin[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

// Comparison Operations Tests

func TestTensorGreater(t *testing.T) {
	t.Skip("Greater not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 5, 3, 2}, Shape{4}, backend)
	b, _ := FromSlice([]float32{2, 3, 3, 4}, Shape{4}, backend)

	result := a.Greater(b)

	expected := []bool{false, true, false, false}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Greater[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorGt(t *testing.T) {
	t.Skip("Gt not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{5, 3}, Shape{2}, backend)
	b, _ := FromSlice([]float32{3, 3}, Shape{2}, backend)

	result := a.Gt(b)
	expected := []bool{true, false}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Gt[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorLower(t *testing.T) {
	t.Skip("Lower not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 5, 3, 2}, Shape{4}, backend)
	b, _ := FromSlice([]float32{2, 3, 3, 4}, Shape{4}, backend)

	result := a.Lower(b)

	expected := []bool{true, false, false, true}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Lower[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorLt(t *testing.T) {
	t.Skip("Lt not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 3}, Shape{2}, backend)
	b, _ := FromSlice([]float32{3, 3}, Shape{2}, backend)

	result := a.Lt(b)
	expected := []bool{true, false}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Lt[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorGreaterEqual(t *testing.T) {
	t.Skip("Greater not implemented in MockBackend")
	t.Skip("GreaterEqual not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 5, 3, 2}, Shape{4}, backend)
	b, _ := FromSlice([]float32{2, 3, 3, 4}, Shape{4}, backend)

	result := a.GreaterEqual(b)

	expected := []bool{false, true, true, false}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("GreaterEqual[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorGe(t *testing.T) {
	t.Skip("Ge not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{5, 3}, Shape{2}, backend)
	b, _ := FromSlice([]float32{3, 3}, Shape{2}, backend)

	result := a.Ge(b)
	expected := []bool{true, true}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Ge[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorLowerEqual(t *testing.T) {
	t.Skip("Lower not implemented in MockBackend")
	t.Skip("LowerEqual not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 5, 3, 2}, Shape{4}, backend)
	b, _ := FromSlice([]float32{2, 3, 3, 4}, Shape{4}, backend)

	result := a.LowerEqual(b)

	expected := []bool{true, false, true, true}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("LowerEqual[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorLe(t *testing.T) {
	t.Skip("Le not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 3}, Shape{2}, backend)
	b, _ := FromSlice([]float32{3, 3}, Shape{2}, backend)

	result := a.Le(b)
	expected := []bool{true, true}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Le[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorEqual(t *testing.T) {
	t.Skip("Equal not implemented in MockBackend")
	t.Skip("Eq not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 5, 3, 2}, Shape{4}, backend)
	b, _ := FromSlice([]float32{2, 3, 3, 4}, Shape{4}, backend)

	result := a.Equal(b)

	expected := []bool{false, false, true, false}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Equal[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorEq(t *testing.T) {
	t.Skip("Eq not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{3, 5}, Shape{2}, backend)
	b, _ := FromSlice([]float32{3, 3}, Shape{2}, backend)

	result := a.Eq(b)
	expected := []bool{true, false}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Eq[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorNotEqual(t *testing.T) {
	t.Skip("NotEqual not implemented in MockBackend")
	t.Skip("Not not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{1, 5, 3, 2}, Shape{4}, backend)
	b, _ := FromSlice([]float32{2, 3, 3, 4}, Shape{4}, backend)

	result := a.NotEqual(b)

	expected := []bool{true, true, false, true}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("NotEqual[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorNe(t *testing.T) {
	t.Skip("Ne not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]float32{3, 5}, Shape{2}, backend)
	b, _ := FromSlice([]float32{3, 3}, Shape{2}, backend)

	result := a.Ne(b)
	expected := []bool{false, true}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Ne[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

// Logical Operations Tests

func TestTensorOr(t *testing.T) {
	t.Skip("Or not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]bool{true, false, true, false}, Shape{4}, backend)
	b, _ := FromSlice([]bool{true, true, false, false}, Shape{4}, backend)

	result := a.Or(b)

	expected := []bool{true, true, true, false}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Or[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorAnd(t *testing.T) {
	t.Skip("And not implemented in MockBackend")
	backend := NewMockBackend()
	a, _ := FromSlice([]bool{true, false, true, false}, Shape{4}, backend)
	b, _ := FromSlice([]bool{true, true, false, false}, Shape{4}, backend)

	result := a.And(b)

	expected := []bool{true, false, false, false}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("And[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorNot(t *testing.T) {
	t.Skip("Not not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]bool{true, false, true, false}, Shape{4}, backend)

	result := tensor.Not()

	expected := []bool{false, true, false, true}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Not[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

// Other Operations Tests

func TestTensorSum(t *testing.T) {
	t.Skip("Sum not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1, 2, 3, 4, 5, 6}, Shape{2, 3}, backend)

	result := tensor.Sum()

	// Sum of all elements: 1+2+3+4+5+6 = 21
	if result.Item() != 21 {
		t.Errorf("Sum() = %v, want 21", result.Item())
	}
}

func TestTensorArgmax(t *testing.T) {
	t.Skip("Argmax not implemented in MockBackend")
	backend := NewMockBackend()
	// [[1, 5, 3],
	//  [9, 2, 7]]
	tensor, _ := FromSlice([]float32{1, 5, 3, 9, 2, 7}, Shape{2, 3}, backend)

	// Argmax along dim 0 (across rows)
	result0 := tensor.Argmax(0)
	assertEqualShape(t, Shape{3}, result0.Shape(), "Argmax(0) shape")
	expected0 := []int64{1, 0, 1} // [row 1 has max in col 0, row 0 has max in col 1, row 1 has max in col 2]
	for i, exp := range expected0 {
		if int64(result0.At(i)) != exp {
			t.Errorf("Argmax(0)[%d] = %v, want %v", i, result0.At(i), exp)
		}
	}

	// Argmax along dim 1 (across columns)
	result1 := tensor.Argmax(1)
	assertEqualShape(t, Shape{2}, result1.Shape(), "Argmax(1) shape")
	expected1 := []int64{1, 0} // [col 1 has max in row 0, col 0 has max in row 1]
	for i, exp := range expected1 {
		if int64(result1.At(i)) != exp {
			t.Errorf("Argmax(1)[%d] = %v, want %v", i, result1.At(i), exp)
		}
	}
}

func TestTensorGather(t *testing.T) {
	t.Skip("Gather not implemented in MockBackend")
	backend := NewMockBackend()
	// [[1, 2],
	//  [3, 4],
	//  [5, 6]]
	tensor, _ := FromSlice([]float32{1, 2, 3, 4, 5, 6}, Shape{3, 2}, backend)

	// Gather rows at indices [0, 2]
	indices, _ := FromSlice([]int32{0, 2}, Shape{2}, backend)
	result := tensor.Gather(0, indices)

	assertEqualShape(t, Shape{2, 2}, result.Shape(), "Gather shape")
	// Should get [[1, 2], [5, 6]]
	assertEqualFloat32(t, 1, result.At(0, 0), "Gather[0,0]")
	assertEqualFloat32(t, 2, result.At(0, 1), "Gather[0,1]")
	assertEqualFloat32(t, 5, result.At(1, 0), "Gather[1,0]")
	assertEqualFloat32(t, 6, result.At(1, 1), "Gather[1,1]")
}

func TestTensorEmbedding(t *testing.T) {
	t.Skip("Embedding not implemented in MockBackend")
	backend := NewMockBackend()
	// Embedding table: 4 words, 3 dimensions
	// [[1, 2, 3],
	//  [4, 5, 6],
	//  [7, 8, 9],
	//  [10, 11, 12]]
	embeddings, _ := FromSlice([]float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, Shape{4, 3}, backend)

	// Lookup indices [0, 2, 1]
	indices, _ := FromSlice([]int32{0, 2, 1}, Shape{3}, backend)
	result := embeddings.Embedding(indices)

	assertEqualShape(t, Shape{3, 3}, result.Shape(), "Embedding shape")
	// Should get:
	// [[1, 2, 3],    <- embedding 0
	//  [7, 8, 9],    <- embedding 2
	//  [4, 5, 6]]    <- embedding 1
	assertEqualFloat32(t, 1, result.At(0, 0), "Embedding[0,0]")
	assertEqualFloat32(t, 7, result.At(1, 0), "Embedding[1,0]")
	assertEqualFloat32(t, 4, result.At(2, 0), "Embedding[2,0]")
}

func TestTensorExpand(t *testing.T) {
	t.Skip("Exp not implemented in MockBackend")
	t.Skip("Expand not implemented in MockBackend")
	backend := NewMockBackend()
	// Shape (2, 1)
	tensor, _ := FromSlice([]float32{1, 2}, Shape{2, 1}, backend)

	// Expand to (2, 3)
	result := tensor.Expand(Shape{2, 3})

	assertEqualShape(t, Shape{2, 3}, result.Shape(), "Expand shape")
	// Should broadcast the values
	// [[1, 1, 1],
	//  [2, 2, 2]]
	assertEqualFloat32(t, 1, result.At(0, 0), "Expand[0,0]")
	assertEqualFloat32(t, 1, result.At(0, 1), "Expand[0,1]")
	assertEqualFloat32(t, 1, result.At(0, 2), "Expand[0,2]")
	assertEqualFloat32(t, 2, result.At(1, 0), "Expand[1,0]")
	assertEqualFloat32(t, 2, result.At(1, 1), "Expand[1,1]")
	assertEqualFloat32(t, 2, result.At(1, 2), "Expand[1,2]")
}

// Type Conversion Tests

func TestTensorInt32(t *testing.T) {
	t.Skip("Cast not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1.7, 2.3, 3.9}, Shape{3}, backend)

	result := tensor.Int32()

	expected := []int32{1, 2, 3}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Int32[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorFloat32(t *testing.T) {
	t.Skip("Cast not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]int32{1, 2, 3}, Shape{3}, backend)

	result := tensor.Float32()

	expected := []float32{1, 2, 3}
	got := result.Data()
	for i := range expected {
		assertEqualFloat32(t, expected[i], got[i], fmt.Sprintf("Float32[%d]", i))
	}
}

func TestTensorFloat64(t *testing.T) {
	t.Skip("Cast not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1.5, 2.5, 3.5}, Shape{3}, backend)

	result := tensor.Float64()

	expected := []float64{1.5, 2.5, 3.5}
	got := result.Data()
	for i := range expected {
		if math.Abs(got[i]-expected[i]) > 1e-6 {
			t.Errorf("Float64[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}

func TestTensorInt64(t *testing.T) {
	t.Skip("Cast not implemented in MockBackend")
	backend := NewMockBackend()
	tensor, _ := FromSlice([]float32{1.7, 2.3, 3.9}, Shape{3}, backend)

	result := tensor.Int64()

	expected := []int64{1, 2, 3}
	got := result.Data()
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Int64[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}
