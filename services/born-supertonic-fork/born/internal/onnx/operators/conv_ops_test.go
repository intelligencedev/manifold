//go:build !wasm

package operators

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

func convF32(t *testing.T, shape tensor.Shape, data []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw: %v", err)
	}
	copy(raw.AsFloat32(), data)
	return raw
}

func convAssertShape(t *testing.T, got *tensor.RawTensor, want tensor.Shape) {
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

func convAssertClose(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d (%v vs %v)", len(got), len(want), got, want)
	}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-4 {
			t.Fatalf("value[%d]: got %v want %v (%v vs %v)", i, got[i], want[i], got, want)
		}
	}
}

func runConv(t *testing.T, attrs []Attribute, inputs ...*tensor.RawTensor) *tensor.RawTensor {
	t.Helper()
	r := NewRegistry()
	out, err := r.Execute(&Context{Backend: cpu.New()}, &Node{OpType: "Conv", Attributes: attrs}, inputs)
	if err != nil {
		t.Fatalf("Conv execute: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("Conv: expected 1 output, got %d", len(out))
	}
	return out[0]
}

func runConvErr(attrs []Attribute, inputs ...*tensor.RawTensor) error {
	r := NewRegistry()
	_, err := r.Execute(&Context{Backend: cpu.New()}, &Node{OpType: "Conv", Attributes: attrs}, inputs)
	return err
}

func cAttrInts(name string, vals ...int64) Attribute { return Attribute{Name: name, Ints: vals} }
func cAttrI(name string, v int64) Attribute          { return Attribute{Name: name, I: v} }
func cAttrS(name, s string) Attribute                { return Attribute{Name: name, S: []byte(s)} }

// --- basic group=1 convolution (cross-correlation) ---

func TestConv_Basic(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 1, 3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 2, 2), cAttrInts("strides", 1, 1)}, x, w)
	convAssertShape(t, out, tensor.Shape{1, 1, 2, 2})
	convAssertClose(t, out.AsFloat32(), []float32{37, 47, 67, 77})
}

func TestConv_WithBias(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 1, 3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	b := convF32(t, tensor.Shape{1}, []float32{10})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 2, 2)}, x, w, b)
	convAssertShape(t, out, tensor.Shape{1, 1, 2, 2})
	convAssertClose(t, out.AsFloat32(), []float32{47, 57, 77, 87})
}

func TestConv_Stride2(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 1, 3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 2, 2), cAttrInts("strides", 2, 2)}, x, w)
	convAssertShape(t, out, tensor.Shape{1, 1, 1, 1})
	convAssertClose(t, out.AsFloat32(), []float32{37})
}

func TestConv_TwoOutputChannels(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 1, 3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})
	// Cout=2: ch0 kernel [1,2,3,4], ch1 kernel all-ones.
	w := convF32(t, tensor.Shape{2, 1, 2, 2}, []float32{1, 2, 3, 4, 1, 1, 1, 1})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 2, 2)}, x, w)
	convAssertShape(t, out, tensor.Shape{1, 2, 2, 2})
	// ch0 = cross-correlation; ch1 = window sums.
	convAssertClose(t, out.AsFloat32(), []float32{37, 47, 67, 77, 12, 16, 24, 28})
}

// --- padding ---

func TestConv_SymmetricPad(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 1, 1, 1})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 2, 2), cAttrInts("pads", 1, 1, 1, 1)}, x, w)
	convAssertShape(t, out, tensor.Shape{1, 1, 3, 3})
	convAssertClose(t, out.AsFloat32(), []float32{1, 3, 2, 4, 10, 6, 3, 7, 4})
}

func TestConv_AsymmetricPad(t *testing.T) {
	// Left pad only: padded row [0,1,2], 1x2 all-ones kernel.
	x := convF32(t, tensor.Shape{1, 1, 1, 2}, []float32{1, 2})
	w := convF32(t, tensor.Shape{1, 1, 1, 2}, []float32{1, 1})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 1, 2), cAttrInts("pads", 0, 1, 0, 0)}, x, w)
	convAssertShape(t, out, tensor.Shape{1, 1, 1, 2})
	convAssertClose(t, out.AsFloat32(), []float32{1, 3})
}

// --- grouped / depthwise ---

func TestConv_Depthwise(t *testing.T) {
	// 2 channels, group=2, each channel its own 2x2 kernel.
	x := convF32(t, tensor.Shape{1, 2, 3, 3}, []float32{
		1, 2, 3, 4, 5, 6, 7, 8, 9, // ch0
		10, 11, 12, 13, 14, 15, 16, 17, 18, // ch1
	})
	w := convF32(t, tensor.Shape{2, 1, 2, 2}, []float32{
		1, 0, 0, 1, // ch0 kernel (diagonal)
		1, 1, 1, 1, // ch1 kernel (sum)
	})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 2, 2), cAttrI("group", 2)}, x, w)
	convAssertShape(t, out, tensor.Shape{1, 2, 2, 2})
	convAssertClose(t, out.AsFloat32(), []float32{6, 8, 12, 14, 48, 52, 60, 64})
}

func TestConv_GroupedTwoChannelsPerGroup(t *testing.T) {
	// Cin=4, group=2 (2 channels/group), Cout=2, 1x1 kernels.
	x := convF32(t, tensor.Shape{1, 4, 1, 1}, []float32{1, 2, 3, 4})
	// out0 over {ch0,ch1} with [1,0]; out1 over {ch2,ch3} with [0,1].
	w := convF32(t, tensor.Shape{2, 2, 1, 1}, []float32{1, 0, 0, 1})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 1, 1), cAttrI("group", 2)}, x, w)
	convAssertShape(t, out, tensor.Shape{1, 2, 1, 1})
	convAssertClose(t, out.AsFloat32(), []float32{1, 4})
}

func TestConv_GroupedBatchN2(t *testing.T) {
	// Same grouped 1x1 setup as above but batch N=2: each sample is split
	// into its own groups, so the per-sample results stay independent.
	x := convF32(t, tensor.Shape{2, 4, 1, 1}, []float32{1, 2, 3, 4, 5, 6, 7, 8})
	w := convF32(t, tensor.Shape{2, 2, 1, 1}, []float32{1, 0, 0, 1})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 1, 1), cAttrI("group", 2)}, x, w)
	convAssertShape(t, out, tensor.Shape{2, 2, 1, 1})
	// sample0 -> {ch0, ch3} = {1, 4}; sample1 -> {ch0, ch3} = {5, 8}.
	convAssertClose(t, out.AsFloat32(), []float32{1, 4, 5, 8})
}

func TestConv_AutoPadValidForcesZeroPads(t *testing.T) {
	// auto_pad=VALID means no padding and overrides the explicit pads
	// attribute, so the result matches an unpadded convolution rather than
	// silently applying pads=[1,1,1,1].
	x := convF32(t, tensor.Shape{1, 1, 3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	out := runConv(t, []Attribute{
		cAttrInts("kernel_shape", 2, 2),
		cAttrInts("pads", 1, 1, 1, 1),
		cAttrS("auto_pad", "VALID"),
	}, x, w)
	convAssertShape(t, out, tensor.Shape{1, 1, 2, 2})
	convAssertClose(t, out.AsFloat32(), []float32{37, 47, 67, 77})
}

// --- errors ---

func TestConv_DilationsRejected(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 1, 3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	if err := runConvErr([]Attribute{cAttrInts("kernel_shape", 2, 2), cAttrInts("dilations", 2, 2)}, x, w); err == nil {
		t.Fatal("expected error for dilations>1, got nil")
	}
}

func TestConv_AutoPadRejected(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 1, 3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	if err := runConvErr([]Attribute{cAttrInts("kernel_shape", 2, 2), cAttrS("auto_pad", "SAME_UPPER")}, x, w); err == nil {
		t.Fatal("expected error for auto_pad=SAME_UPPER, got nil")
	}
}

func TestConv_NonSquareStrideRejected(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 1, 3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	if err := runConvErr([]Attribute{cAttrInts("kernel_shape", 2, 2), cAttrInts("strides", 2, 1)}, x, w); err == nil {
		t.Fatal("expected error for non-square stride, got nil")
	}
}

func TestConv_Non4DInput(t *testing.T) {
	x := convF32(t, tensor.Shape{3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	if err := runConvErr([]Attribute{cAttrInts("kernel_shape", 2, 2)}, x, w); err == nil {
		t.Fatal("expected error for non-4D input, got nil")
	}
}
