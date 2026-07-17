//go:build !wasm

package operators

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

func poolTensor(t *testing.T, shape tensor.Shape, data []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw: %v", err)
	}
	copy(raw.AsFloat32(), data)
	return raw
}

func poolAssertShape(t *testing.T, got *tensor.RawTensor, want tensor.Shape) {
	t.Helper()
	gs := got.Shape()
	if len(gs) != len(want) {
		t.Fatalf("shape rank: got %v want %v", gs, want)
	}
	for i := range want {
		if gs[i] != want[i] {
			t.Fatalf("shape: got %v want %v", gs, want)
		}
	}
}

func poolAssertClose(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d (%v vs %v)", len(got), len(want), got, want)
	}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-5 {
			t.Fatalf("value[%d]: got %v want %v (%v vs %v)", i, got[i], want[i], got, want)
		}
	}
}

func runPool(t *testing.T, op string, attrs []Attribute, in *tensor.RawTensor) *tensor.RawTensor {
	t.Helper()
	r := NewRegistry()
	out, err := r.Execute(&Context{Backend: cpu.New()}, &Node{OpType: op, Attributes: attrs}, []*tensor.RawTensor{in})
	if err != nil {
		t.Fatalf("%s execute: %v", op, err)
	}
	if len(out) != 1 {
		t.Fatalf("%s: expected 1 output, got %d", op, len(out))
	}
	return out[0]
}

func runPoolErr(op string, attrs []Attribute, in *tensor.RawTensor) error {
	r := NewRegistry()
	_, err := r.Execute(&Context{Backend: cpu.New()}, &Node{OpType: op, Attributes: attrs}, []*tensor.RawTensor{in})
	return err
}

func ints(name string, vals ...int64) Attribute { return Attribute{Name: name, Ints: vals} }
func iattr(name string, v int64) Attribute      { return Attribute{Name: name, I: v} }

// --- MaxPool ---

func TestMaxPool_1x2Window(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 1, 4}, []float32{1, 2, 3, 4})
	out := runPool(t, "MaxPool", []Attribute{ints("kernel_shape", 1, 2), ints("strides", 1, 2)}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 2})
	poolAssertClose(t, out.AsFloat32(), []float32{2, 4})
}

func TestMaxPool_2x2(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	out := runPool(t, "MaxPool", []Attribute{ints("kernel_shape", 2, 2), ints("strides", 2, 2)}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 1})
	poolAssertClose(t, out.AsFloat32(), []float32{4})
}

func TestMaxPool_TwoChannels(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 2, 1, 2}, []float32{1, 2, 3, 4})
	out := runPool(t, "MaxPool", []Attribute{ints("kernel_shape", 1, 2), ints("strides", 1, 2)}, in)
	poolAssertShape(t, out, tensor.Shape{1, 2, 1, 1})
	poolAssertClose(t, out.AsFloat32(), []float32{2, 4})
}

func TestMaxPool_PadsExcludedFromMax(t *testing.T) {
	// inW=3, kernel 2, stride 2, right pad 1 -> outW=2; padded cell must not win.
	in := poolTensor(t, tensor.Shape{1, 1, 1, 3}, []float32{1, 2, 3})
	out := runPool(t, "MaxPool", []Attribute{
		ints("kernel_shape", 1, 2), ints("strides", 1, 2), ints("pads", 0, 0, 0, 1),
	}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 2})
	poolAssertClose(t, out.AsFloat32(), []float32{2, 3})
}

// --- AveragePool ---

func TestAveragePool_1x2Window(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 1, 4}, []float32{1, 2, 3, 4})
	out := runPool(t, "AveragePool", []Attribute{ints("kernel_shape", 1, 2), ints("strides", 1, 2)}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 2})
	poolAssertClose(t, out.AsFloat32(), []float32{1.5, 3.5})
}

func TestAveragePool_2x2(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	out := runPool(t, "AveragePool", []Attribute{ints("kernel_shape", 2, 2), ints("strides", 2, 2)}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 1})
	poolAssertClose(t, out.AsFloat32(), []float32{2.5})
}

func TestAveragePool_CountExcludePad(t *testing.T) {
	// Default count_include_pad=0: padded cell excluded from the denominator.
	in := poolTensor(t, tensor.Shape{1, 1, 1, 3}, []float32{1, 2, 3})
	out := runPool(t, "AveragePool", []Attribute{
		ints("kernel_shape", 1, 2), ints("strides", 1, 2), ints("pads", 0, 0, 0, 1),
	}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 2})
	poolAssertClose(t, out.AsFloat32(), []float32{1.5, 3})
}

func TestAveragePool_CountIncludePad(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 1, 3}, []float32{1, 2, 3})
	out := runPool(t, "AveragePool", []Attribute{
		ints("kernel_shape", 1, 2), ints("strides", 1, 2), ints("pads", 0, 0, 0, 1),
		iattr("count_include_pad", 1),
	}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 2})
	poolAssertClose(t, out.AsFloat32(), []float32{1.5, 1.5})
}

// --- errors ---

func TestPool_Non4DInput(t *testing.T) {
	in := poolTensor(t, tensor.Shape{4}, []float32{1, 2, 3, 4})
	if err := runPoolErr("MaxPool", []Attribute{ints("kernel_shape", 1, 2)}, in); err == nil {
		t.Fatal("expected error for non-4D input, got nil")
	}
}

func sattr(name, val string) Attribute { return Attribute{Name: name, S: []byte(val)} }

// --- larger / batched / stride variants ---

func TestMaxPool_4x4Stride2(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 4, 4}, []float32{
		1, 2, 3, 4,
		5, 6, 7, 8,
		9, 10, 11, 12,
		13, 14, 15, 16,
	})
	out := runPool(t, "MaxPool", []Attribute{ints("kernel_shape", 2, 2), ints("strides", 2, 2)}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 2, 2})
	poolAssertClose(t, out.AsFloat32(), []float32{6, 8, 14, 16})
}

func TestMaxPool_BatchN2(t *testing.T) {
	in := poolTensor(t, tensor.Shape{2, 1, 1, 2}, []float32{1, 2, 3, 4})
	out := runPool(t, "MaxPool", []Attribute{ints("kernel_shape", 1, 2), ints("strides", 1, 2)}, in)
	poolAssertShape(t, out, tensor.Shape{2, 1, 1, 1})
	poolAssertClose(t, out.AsFloat32(), []float32{2, 4})
}

func TestMaxPool_DefaultStrides(t *testing.T) {
	// No strides attribute -> default 1,1 -> overlapping output of width 3.
	in := poolTensor(t, tensor.Shape{1, 1, 1, 4}, []float32{1, 2, 3, 4})
	out := runPool(t, "MaxPool", []Attribute{ints("kernel_shape", 1, 2)}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 3})
	poolAssertClose(t, out.AsFloat32(), []float32{2, 3, 4})
}

func TestMaxPool_OverlappingStride(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 1, 4}, []float32{1, 3, 2, 4})
	out := runPool(t, "MaxPool", []Attribute{ints("kernel_shape", 1, 2), ints("strides", 1, 1)}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 3})
	poolAssertClose(t, out.AsFloat32(), []float32{3, 3, 4})
}

func TestAveragePool_LeftPadExcluded(t *testing.T) {
	// Left pad makes wStart negative for the first window; padded cell skipped.
	in := poolTensor(t, tensor.Shape{1, 1, 1, 2}, []float32{2, 4})
	out := runPool(t, "AveragePool", []Attribute{
		ints("kernel_shape", 1, 2), ints("strides", 1, 1), ints("pads", 0, 1, 0, 0),
	}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 2})
	poolAssertClose(t, out.AsFloat32(), []float32{2, 3})
}

// --- unsupported attributes are rejected, not silently mispooled ---

func TestPool_CeilModeRejected(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 1, 4}, []float32{1, 2, 3, 4})
	if err := runPoolErr("MaxPool", []Attribute{ints("kernel_shape", 1, 2), iattr("ceil_mode", 1)}, in); err == nil {
		t.Fatal("expected error for ceil_mode=1, got nil")
	}
}

func TestPool_AutoPadRejected(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 1, 4}, []float32{1, 2, 3, 4})
	if err := runPoolErr("MaxPool", []Attribute{ints("kernel_shape", 1, 2), sattr("auto_pad", "SAME_UPPER")}, in); err == nil {
		t.Fatal("expected error for auto_pad=SAME_UPPER, got nil")
	}
}

func TestPool_DilationsRejected(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 1, 4}, []float32{1, 2, 3, 4})
	if err := runPoolErr("AveragePool", []Attribute{ints("kernel_shape", 1, 2), ints("dilations", 2, 2)}, in); err == nil {
		t.Fatal("expected error for dilations>1, got nil")
	}
}

func TestPool_BadKernelShape(t *testing.T) {
	in := poolTensor(t, tensor.Shape{1, 1, 1, 4}, []float32{1, 2, 3, 4})
	if err := runPoolErr("MaxPool", []Attribute{ints("kernel_shape", 2)}, in); err == nil {
		t.Fatal("expected error for 1D kernel_shape, got nil")
	}
}

func TestMaxPool_NonSquareKernel(t *testing.T) {
	// 3x1 vertical window over a [1,1,3,2] input: pool down each column.
	// rows [1,2] [3,4] [5,6] -> outH=1, outW=2 -> column maxima [5,6].
	in := poolTensor(t, tensor.Shape{1, 1, 3, 2}, []float32{1, 2, 3, 4, 5, 6})
	out := runPool(t, "MaxPool", []Attribute{ints("kernel_shape", 3, 1), ints("strides", 1, 1)}, in)
	poolAssertShape(t, out, tensor.Shape{1, 1, 1, 2})
	poolAssertClose(t, out.AsFloat32(), []float32{5, 6})
}

func TestPool_NegativeOutputDim(t *testing.T) {
	// Kernel larger than the input with no padding yields a non-positive
	// output dim, which must be rejected rather than producing a 0-size or
	// negative-shape tensor.
	in := poolTensor(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	if err := runPoolErr("MaxPool", []Attribute{ints("kernel_shape", 3, 3)}, in); err == nil {
		t.Fatal("expected error for kernel larger than input, got nil")
	}
}

func TestMaxPool_IndicesOutputRejected(t *testing.T) {
	// A second (Indices) output is optional in ONNX MaxPool but unsupported
	// here; requesting it must error instead of silently dropping it.
	in := poolTensor(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	r := NewRegistry()
	node := &Node{
		OpType:     "MaxPool",
		Attributes: []Attribute{ints("kernel_shape", 2, 2)},
		Outputs:    []string{"y", "indices"},
	}
	if _, err := r.Execute(&Context{Backend: cpu.New()}, node, []*tensor.RawTensor{in}); err == nil {
		t.Fatal("expected error when MaxPool Indices output is requested, got nil")
	}
}
