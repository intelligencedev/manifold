//go:build !wasm

package operators

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// --- test helpers ---

func f32Tensor(t *testing.T, shape tensor.Shape, data []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw float32: %v", err)
	}
	copy(raw.AsFloat32(), data)
	return raw
}

func i64Tensor(t *testing.T, data []int64) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(tensor.Shape{len(data)}, tensor.Int64, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw int64: %v", err)
	}
	copy(raw.AsInt64(), data)
	return raw
}

func assertShape(t *testing.T, got *tensor.RawTensor, want tensor.Shape) {
	t.Helper()
	gs := got.Shape()
	if len(gs) != len(want) {
		t.Fatalf("shape rank: got %v, want %v", gs, want)
	}
	for i := range want {
		if gs[i] != want[i] {
			t.Fatalf("shape: got %v, want %v", gs, want)
		}
	}
}

func assertClose(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d (%v vs %v)", len(got), len(want), got, want)
	}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-5 {
			t.Fatalf("value[%d]: got %v, want %v (%v vs %v)", i, got[i], want[i], got, want)
		}
	}
}

func execOp(t *testing.T, opType string, attrs []Attribute, inputs ...*tensor.RawTensor) *tensor.RawTensor {
	t.Helper()
	r := NewRegistry()
	ctx := &Context{Backend: cpu.New()}
	node := &Node{OpType: opType, Attributes: attrs}
	out, err := r.Execute(ctx, node, inputs)
	if err != nil {
		t.Fatalf("%s execute: %v", opType, err)
	}
	if len(out) != 1 {
		t.Fatalf("%s: expected 1 output, got %d", opType, len(out))
	}
	return out[0]
}

// --- ReduceMean ---

func TestReduceMean_SingleAxisNoKeepDims(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	axes := i64Tensor(t, []int64{1})
	out := execOp(t, "ReduceMean", []Attribute{{Name: "keepdims", I: 0}}, data, axes)
	assertShape(t, out, tensor.Shape{2})
	assertClose(t, out.AsFloat32(), []float32{2, 5})
}

func TestReduceMean_SingleAxisKeepDims(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	axes := i64Tensor(t, []int64{1})
	out := execOp(t, "ReduceMean", []Attribute{{Name: "keepdims", I: 1}}, data, axes)
	assertShape(t, out, tensor.Shape{2, 1})
	assertClose(t, out.AsFloat32(), []float32{2, 5})
}

func TestReduceMean_MultiAxis(t *testing.T) {
	// [2,2,2], reduce axes {1,2} -> mean of each 4-element block
	data := f32Tensor(t, tensor.Shape{2, 2, 2}, []float32{1, 2, 3, 4, 5, 6, 7, 8})
	axes := i64Tensor(t, []int64{1, 2})
	out := execOp(t, "ReduceMean", []Attribute{{Name: "keepdims", I: 0}}, data, axes)
	assertShape(t, out, tensor.Shape{2})
	assertClose(t, out.AsFloat32(), []float32{2.5, 6.5})
}

func TestReduceMean_NegativeAxis(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	axes := i64Tensor(t, []int64{-1})
	out := execOp(t, "ReduceMean", []Attribute{{Name: "keepdims", I: 0}}, data, axes)
	assertShape(t, out, tensor.Shape{2})
	assertClose(t, out.AsFloat32(), []float32{2, 5})
}

// --- ReduceMax / ReduceMin ---

func TestReduceMax_SingleAxisKeepDims(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 6, 5, 4})
	axes := i64Tensor(t, []int64{1})
	out := execOp(t, "ReduceMax", []Attribute{{Name: "keepdims", I: 1}}, data, axes)
	assertShape(t, out, tensor.Shape{2, 1})
	assertClose(t, out.AsFloat32(), []float32{3, 6})
}

func TestReduceMin_SingleAxisNoKeepDims(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 6, 5, 4})
	axes := i64Tensor(t, []int64{1})
	out := execOp(t, "ReduceMin", []Attribute{{Name: "keepdims", I: 0}}, data, axes)
	assertShape(t, out, tensor.Shape{2})
	assertClose(t, out.AsFloat32(), []float32{1, 4})
}

// --- Pow ---

func TestPow_SquareScalarExponent(t *testing.T) {
	base := f32Tensor(t, tensor.Shape{2, 2}, []float32{1, 2, 3, 4})
	exp := f32Tensor(t, tensor.Shape{}, []float32{2})
	out := execOp(t, "Pow", nil, base, exp)
	assertShape(t, out, tensor.Shape{2, 2})
	assertClose(t, out.AsFloat32(), []float32{1, 4, 9, 16})
}

func TestPow_SqrtScalarExponent(t *testing.T) {
	base := f32Tensor(t, tensor.Shape{4}, []float32{1, 4, 9, 16})
	exp := f32Tensor(t, tensor.Shape{1}, []float32{0.5})
	out := execOp(t, "Pow", nil, base, exp)
	assertShape(t, out, tensor.Shape{4})
	assertClose(t, out.AsFloat32(), []float32{1, 2, 3, 4})
}

func TestPow_ElementwiseExponent(t *testing.T) {
	base := f32Tensor(t, tensor.Shape{3}, []float32{2, 3, 4})
	exp := f32Tensor(t, tensor.Shape{3}, []float32{1, 2, 3})
	out := execOp(t, "Pow", nil, base, exp)
	assertShape(t, out, tensor.Shape{3})
	assertClose(t, out.AsFloat32(), []float32{2, 9, 64})
}

func TestPow_NegativeBase(t *testing.T) {
	// Integer exponents on a negative base stay real: (-2)^3 = -8, (-3)^2 = 9.
	base := f32Tensor(t, tensor.Shape{2}, []float32{-2, -3})
	exp := f32Tensor(t, tensor.Shape{2}, []float32{3, 2})
	out := execOp(t, "Pow", nil, base, exp)
	assertShape(t, out, tensor.Shape{2})
	assertClose(t, out.AsFloat32(), []float32{-8, 9})
}

// --- 4D NCHW reduction: the SE-pooling shape the real model uses ---

func TestReduceMean_NCHWSqueezeExcite(t *testing.T) {
	// [N=1, C=2, H=2, W=2]: channel 0 spatial mean = 2.5, channel 1 = 6.5.
	data := f32Tensor(t, tensor.Shape{1, 2, 2, 2}, []float32{1, 2, 3, 4, 5, 6, 7, 8})
	axes := i64Tensor(t, []int64{2, 3})
	out := execOp(t, "ReduceMean", []Attribute{{Name: "keepdims", I: 1}}, data, axes)
	assertShape(t, out, tensor.Shape{1, 2, 1, 1})
	assertClose(t, out.AsFloat32(), []float32{2.5, 6.5})
}

func TestReduceMax_NegativeMultiAxis(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{1, 2, 2, 2}, []float32{1, 2, 3, 4, 5, 6, 7, 8})
	axes := i64Tensor(t, []int64{-2, -1})
	out := execOp(t, "ReduceMax", []Attribute{{Name: "keepdims", I: 0}}, data, axes)
	assertShape(t, out, tensor.Shape{1, 2})
	assertClose(t, out.AsFloat32(), []float32{4, 8})
}

// --- axes via attribute (older opset) and empty-axes semantics ---

func TestReduceMean_AxesAsAttribute(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	out := execOp(t, "ReduceMean", []Attribute{
		{Name: "keepdims", I: 0},
		{Name: "axes", Ints: []int64{1}},
	}, data)
	assertShape(t, out, tensor.Shape{2})
	assertClose(t, out.AsFloat32(), []float32{2, 5})
}

func TestReduceMean_NoopEmptyAxes(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	out := execOp(t, "ReduceMean", []Attribute{
		{Name: "keepdims", I: 0},
		{Name: "noop_with_empty_axes", I: 1},
	}, data)
	assertShape(t, out, tensor.Shape{2, 3})
	assertClose(t, out.AsFloat32(), []float32{1, 2, 3, 4, 5, 6})
}

func TestReduceMean_AllAxesWhenEmpty(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	out := execOp(t, "ReduceMean", []Attribute{{Name: "keepdims", I: 0}}, data)
	assertShape(t, out, tensor.Shape{})
	assertClose(t, out.AsFloat32(), []float32{3.5})
}

func TestReduceMax_AllAxesWhenEmpty(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	out := execOp(t, "ReduceMax", []Attribute{{Name: "keepdims", I: 0}}, data)
	assertShape(t, out, tensor.Shape{})
	assertClose(t, out.AsFloat32(), []float32{6})
}

func TestReduceMin_AllAxesWhenEmpty(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	out := execOp(t, "ReduceMin", []Attribute{{Name: "keepdims", I: 0}}, data)
	assertShape(t, out, tensor.Shape{})
	assertClose(t, out.AsFloat32(), []float32{1})
}

// --- error paths ---

func execOpErr(opType string, attrs []Attribute, inputs ...*tensor.RawTensor) error {
	r := NewRegistry()
	ctx := &Context{Backend: cpu.New()}
	node := &Node{OpType: opType, Attributes: attrs}
	_, err := r.Execute(ctx, node, inputs)
	return err
}

func TestReduce_AxisOutOfRange(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	axes := i64Tensor(t, []int64{5})
	if err := execOpErr("ReduceMean", []Attribute{{Name: "keepdims", I: 0}}, data, axes); err == nil {
		t.Fatal("expected error for axis out of range, got nil")
	}
}

func TestReduce_NonFloat32Data(t *testing.T) {
	data := i64Tensor(t, []int64{1, 2, 3, 4, 5, 6})
	axes := i64Tensor(t, []int64{0})
	if err := execOpErr("ReduceMean", []Attribute{{Name: "keepdims", I: 0}}, data, axes); err == nil {
		t.Fatal("expected error for non-float32 data, got nil")
	}
}

func TestReduce_NonInt64Axes(t *testing.T) {
	data := f32Tensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	raw, err := tensor.NewRaw(tensor.Shape{1}, tensor.Int32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw int32: %v", err)
	}
	raw.AsInt32()[0] = 1
	if err := execOpErr("ReduceMean", []Attribute{{Name: "keepdims", I: 0}}, data, raw); err == nil {
		t.Fatal("expected error for non-int64 axes, got nil")
	}
}

func TestPow_WrongInputCount(t *testing.T) {
	base := f32Tensor(t, tensor.Shape{4}, []float32{1, 2, 3, 4})
	if err := execOpErr("Pow", nil, base); err == nil {
		t.Fatal("expected error for wrong input count, got nil")
	}
}

func TestPow_ExponentLengthMismatch(t *testing.T) {
	base := f32Tensor(t, tensor.Shape{4}, []float32{1, 2, 3, 4})
	exp := f32Tensor(t, tensor.Shape{2}, []float32{2, 3})
	if err := execOpErr("Pow", nil, base, exp); err == nil {
		t.Fatal("expected error for exponent length mismatch, got nil")
	}
}

func TestPow_NilInput(t *testing.T) {
	base := f32Tensor(t, tensor.Shape{4}, []float32{1, 2, 3, 4})
	if err := execOpErr("Pow", nil, base, nil); err == nil {
		t.Fatal("expected error for nil input, got nil")
	}
}
