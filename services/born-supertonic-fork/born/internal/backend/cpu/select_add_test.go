package cpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// newFloat32Tensor creates a float32 tensor on CPU with the given shape and data.
func newFloat32Tensor(t *testing.T, shape tensor.Shape, data []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("newFloat32Tensor: failed to create tensor: %v", err)
	}
	dst := raw.AsFloat32()
	copy(dst, data)
	return raw
}

// newInt32Tensor creates an int32 tensor on CPU with the given shape and data.
func newInt32Tensor(t *testing.T, shape tensor.Shape, data []int32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Int32, tensor.CPU)
	if err != nil {
		t.Fatalf("newInt32Tensor: failed to create tensor: %v", err)
	}
	dst := raw.AsInt32()
	copy(dst, data)
	return raw
}

// TestSelectAdd_Dim0_UniqueIndices tests scatter-add along dim=0 with distinct indices.
// Each source row lands at a different destination row — no accumulation.
func TestSelectAdd_Dim0_UniqueIndices(t *testing.T) {
	b := New()

	// dest: zeros [3, 2]
	dest := newFloat32Tensor(t, tensor.Shape{3, 2}, []float32{0, 0, 0, 0, 0, 0})

	// indices: [2, 0] — row 0 of src goes to dest row 2, row 1 of src goes to dest row 0.
	indices := newInt32Tensor(t, tensor.Shape{2}, []int32{2, 0})

	// src: [[1, 2], [3, 4]]
	src := newFloat32Tensor(t, tensor.Shape{2, 2}, []float32{1, 2, 3, 4})

	got := b.SelectAdd(dest, 0, indices, src)

	// Expected dest: [[3,4], [0,0], [1,2]]
	want := []float32{3, 4, 0, 0, 1, 2}
	gotData := got.AsFloat32()
	for i, w := range want {
		if gotData[i] != w {
			t.Errorf("SelectAdd unique indices: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}

	// Verify dest is not modified (immutability).
	destData := dest.AsFloat32()
	for i, v := range destData {
		if v != 0 {
			t.Errorf("SelectAdd: dest was modified at index %d (got %v)", i, v)
		}
	}
}

// TestSelectAdd_Dim0_DuplicateIndices tests accumulation when two source rows
// map to the same destination row (the critical Embedding backward case).
func TestSelectAdd_Dim0_DuplicateIndices(t *testing.T) {
	b := New()

	// Embedding backward scenario:
	//   weight:  [4, 2]  (vocab=4, dim=2), dest is zero-filled grad
	//   indices: [0, 1, 0] — token 0 appears twice
	//   src:     [[1,2],[3,4],[5,6]] — upstream grad output

	dest := newFloat32Tensor(t, tensor.Shape{4, 2}, []float32{
		0, 0,
		0, 0,
		0, 0,
		0, 0,
	})
	indices := newInt32Tensor(t, tensor.Shape{3}, []int32{0, 1, 0})
	src := newFloat32Tensor(t, tensor.Shape{3, 2}, []float32{
		1, 2,
		3, 4,
		5, 6,
	})

	got := b.SelectAdd(dest, 0, indices, src)
	gotData := got.AsFloat32()

	// Expected:
	//   row 0: [1,2] + [5,6] = [6,8]   (index 0 appears twice)
	//   row 1: [3,4]
	//   rows 2,3: [0,0]
	want := []float32{6, 8, 3, 4, 0, 0, 0, 0}
	for i, w := range want {
		if gotData[i] != w {
			t.Errorf("SelectAdd duplicate indices: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestSelectAdd_Dim0_AccumulatesIntoNonZeroDest verifies that SelectAdd adds onto
// an existing non-zero dest rather than overwriting.
func TestSelectAdd_Dim0_AccumulatesIntoNonZeroDest(t *testing.T) {
	b := New()

	// dest has existing values.
	dest := newFloat32Tensor(t, tensor.Shape{2, 2}, []float32{10, 20, 30, 40})
	indices := newInt32Tensor(t, tensor.Shape{1}, []int32{0})
	src := newFloat32Tensor(t, tensor.Shape{1, 2}, []float32{1, 2})

	got := b.SelectAdd(dest, 0, indices, src)
	gotData := got.AsFloat32()

	// Expected: row 0 += [1,2] → [11,22]; row 1 unchanged → [30,40].
	want := []float32{11, 22, 30, 40}
	for i, w := range want {
		if gotData[i] != w {
			t.Errorf("SelectAdd non-zero dest: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestSelectAdd_Dim0_Float64 verifies float64 support.
func TestSelectAdd_Dim0_Float64(t *testing.T) {
	b := New()

	dest, err := tensor.NewRaw(tensor.Shape{3, 2}, tensor.Float64, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw: %v", err)
	}

	indices := newInt32Tensor(t, tensor.Shape{2}, []int32{1, 1})

	src, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float64, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw: %v", err)
	}
	srcData := src.AsFloat64()
	srcData[0], srcData[1] = 1.5, 2.5
	srcData[2], srcData[3] = 3.5, 4.5

	got := b.SelectAdd(dest, 0, indices, src)
	gotData := got.AsFloat64()

	// index 1 appears twice: row 1 = [1.5+3.5, 2.5+4.5] = [5.0, 7.0]
	want := []float64{0, 0, 5.0, 7.0, 0, 0}
	for i, w := range want {
		if gotData[i] != w {
			t.Errorf("SelectAdd float64: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestSelectAdd_NegativeDim verifies that negative dim values are normalised correctly.
func TestSelectAdd_NegativeDim(t *testing.T) {
	b := New()

	dest := newFloat32Tensor(t, tensor.Shape{3, 4}, []float32{
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0,
	})
	indices := newInt32Tensor(t, tensor.Shape{2}, []int32{2, 0})
	src := newFloat32Tensor(t, tensor.Shape{2, 4}, []float32{
		1, 2, 3, 4,
		5, 6, 7, 8,
	})

	// dim=-2 is equivalent to dim=0 for a 2D tensor.
	got := b.SelectAdd(dest, -2, indices, src)
	gotData := got.AsFloat32()

	// row 2 ← src[0] = [1,2,3,4]; row 0 ← src[1] = [5,6,7,8]
	want := []float32{5, 6, 7, 8, 0, 0, 0, 0, 1, 2, 3, 4}
	for i, w := range want {
		if gotData[i] != w {
			t.Errorf("SelectAdd negative dim: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestSelectAdd_PanicOnOutOfBoundsIndex verifies that an out-of-bounds index panics.
func TestSelectAdd_PanicOnOutOfBoundsIndex(t *testing.T) {
	b := New()

	dest := newFloat32Tensor(t, tensor.Shape{3, 2}, []float32{0, 0, 0, 0, 0, 0})
	indices := newInt32Tensor(t, tensor.Shape{1}, []int32{5}) // 5 >= vocabSize=3
	src := newFloat32Tensor(t, tensor.Shape{1, 2}, []float32{1, 2})

	defer func() {
		if r := recover(); r == nil {
			t.Error("SelectAdd: expected panic for out-of-bounds index, got none")
		}
	}()

	b.SelectAdd(dest, 0, indices, src)
}

// TestSelectAdd_PanicOnWrongIndexDType verifies that non-int32 indices panic.
func TestSelectAdd_PanicOnWrongIndexDType(t *testing.T) {
	b := New()

	dest := newFloat32Tensor(t, tensor.Shape{3, 2}, make([]float32, 6))
	src := newFloat32Tensor(t, tensor.Shape{1, 2}, []float32{1, 2})

	// Float32 indices should panic.
	floatIdx, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw: %v", err)
	}
	floatIdx.AsFloat32()[0] = 0

	defer func() {
		if r := recover(); r == nil {
			t.Error("SelectAdd: expected panic for non-int32 indices, got none")
		}
	}()

	b.SelectAdd(dest, 0, floatIdx, src)
}
