package tensor

import (
	"math"
	"testing"
)

// TestSliceReverseFullAxis covers ONNX-style full-axis reversal:
// starts=[-1], ends=[INT64_MIN], steps=[-1]. The result must keep every
// element (reversed), not drop index 0. This is the reverse used by the
// BirdNET v2.4 spectrogram front-end.
func TestSliceReverseFullAxis(t *testing.T) {
	x, err := NewRaw(Shape{5}, Float32, CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(x.AsFloat32(), []float32{0, 1, 2, 3, 4})

	out, err := Slice(x, []int64{-1}, []int64{math.MinInt64}, []int64{0}, []int64{-1})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Shape(); len(got) != 1 || got[0] != 5 {
		t.Fatalf("shape: got %v, want [5]", got)
	}
	want := []float32{4, 3, 2, 1, 0}
	got := out.AsFloat32()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestSliceReverseLastAxis2D reverses the last axis of a 2D tensor, matching
// the model's [1, N] reverse; the axis length must be preserved.
func TestSliceReverseLastAxis2D(t *testing.T) {
	x, err := NewRaw(Shape{1, 4}, Float32, CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(x.AsFloat32(), []float32{10, 20, 30, 40})

	out, err := Slice(x, []int64{-1}, []int64{math.MinInt64}, []int64{-1}, []int64{-1})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Shape(); len(got) != 2 || got[0] != 1 || got[1] != 4 {
		t.Fatalf("shape: got %v, want [1 4]", got)
	}
	want := []float32{40, 30, 20, 10}
	got := out.AsFloat32()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestSliceReversePartial reverses a sub-range: start=3, end=0 (exclusive),
// step=-1 -> indices 3,2,1.
func TestSliceReversePartial(t *testing.T) {
	x, err := NewRaw(Shape{5}, Float32, CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(x.AsFloat32(), []float32{0, 1, 2, 3, 4})

	out, err := Slice(x, []int64{3}, []int64{0}, []int64{0}, []int64{-1})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Shape(); len(got) != 1 || got[0] != 3 {
		t.Fatalf("shape: got %v, want [3]", got)
	}
	want := []float32{3, 2, 1}
	got := out.AsFloat32()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestSliceReverseStepTwo covers a non-unit reverse stride: starts=[4],
// ends=[0] (exclusive), steps=[-2] on dim=5 visits indices 4 and 2, so the
// length formula and the -step divisor must produce [4, 2].
func TestSliceReverseStepTwo(t *testing.T) {
	x, err := NewRaw(Shape{5}, Float32, CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(x.AsFloat32(), []float32{0, 1, 2, 3, 4})

	out, err := Slice(x, []int64{4}, []int64{0}, []int64{0}, []int64{-2})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Shape(); len(got) != 1 || got[0] != 2 {
		t.Fatalf("shape: got %v, want [2]", got)
	}
	want := []float32{4, 2}
	got := out.AsFloat32()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestSliceZeroStepErrors locks in the guard: step 0 is invalid (ONNX) and
// previously caused a divide-by-zero panic in the length formula.
func TestSliceZeroStepErrors(t *testing.T) {
	x, err := NewRaw(Shape{5}, Float32, CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(x.AsFloat32(), []float32{0, 1, 2, 3, 4})
	if _, err := Slice(x, []int64{0}, []int64{5}, []int64{0}, []int64{0}); err == nil {
		t.Fatal("expected error for step == 0, got nil")
	}
}

// TestSliceForwardUnchanged guards the positive-step path against regressions.
func TestSliceForwardUnchanged(t *testing.T) {
	x, err := NewRaw(Shape{5}, Float32, CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(x.AsFloat32(), []float32{0, 1, 2, 3, 4})

	out, err := Slice(x, []int64{1}, []int64{4}, []int64{0}, []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Shape(); len(got) != 1 || got[0] != 3 {
		t.Fatalf("shape: got %v, want [3]", got)
	}
	want := []float32{1, 2, 3}
	got := out.AsFloat32()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestSliceLengthMismatchErrors locks in the parameter-length guard: starts,
// ends, steps and axes must align, otherwise Slice returns an error rather than
// indexing a parallel slice out of range. Here ends is shorter than the
// defaulted axes (len 2).
func TestSliceLengthMismatchErrors(t *testing.T) {
	x, err := NewRaw(Shape{5}, Float32, CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(x.AsFloat32(), []float32{0, 1, 2, 3, 4})
	if _, err := Slice(x, []int64{0, 1}, []int64{3}, nil, nil); err == nil {
		t.Fatal("expected error for mismatched starts/ends lengths, got nil")
	}
}
