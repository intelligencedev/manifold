//go:build !wasm

package operators

import (
	"math"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

func stF32(t *testing.T, shape tensor.Shape, data []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw f32: %v", err)
	}
	copy(raw.AsFloat32(), data)
	return raw
}

func stI64(t *testing.T, shape tensor.Shape, data []int64) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Int64, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw i64: %v", err)
	}
	copy(raw.AsInt64(), data)
	return raw
}

func stExec(t *testing.T, node *Node, inputs ...*tensor.RawTensor) *tensor.RawTensor {
	t.Helper()
	r := NewRegistry()
	out, err := r.Execute(&Context{Backend: cpu.New()}, node, inputs)
	if err != nil {
		t.Fatalf("%s execute: %v", node.OpType, err)
	}
	if len(out) != 1 {
		t.Fatalf("%s: expected 1 output, got %d", node.OpType, len(out))
	}
	return out[0]
}

func stAssertShape(t *testing.T, got *tensor.RawTensor, want tensor.Shape) {
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

func stAssertClose(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-4 {
			t.Fatalf("value[%d]: got %v want %v\n got=%v\nwant=%v", i, got[i], want[i], got, want)
		}
	}
}

// ConstantOfShape must honor the dtype of its "value" tensor attribute.
// Regression: Born hardcoded float32, breaking int64 shape arithmetic in the
// Supertonic graphs (float32 x int64 Mul).
func TestConstantOfShapeInt64(t *testing.T) {
	shape := stI64(t, tensor.Shape{1}, []int64{3})
	val := stI64(t, tensor.Shape{1}, []int64{5})
	out := stExec(t, &Node{OpType: "ConstantOfShape", Attributes: []Attribute{{Name: "value", T: val}}}, shape)
	if out.DType() != tensor.Int64 {
		t.Fatalf("dtype = %s, want int64", out.DType())
	}
	stAssertShape(t, out, tensor.Shape{3})
	got := out.AsInt64()
	if len(got) != 3 || got[0] != 5 || got[1] != 5 || got[2] != 5 {
		t.Fatalf("got %v, want [5 5 5]", got)
	}
}

// Expand uses ONNX bidirectional broadcasting: a target dim of 1 keeps the
// (possibly larger) input dim. Regression: Born errored on 64 -> 1.
func TestExpandBroadcast(t *testing.T) {
	in := stF32(t, tensor.Shape{2, 1}, []float32{1, 2})
	shp := stI64(t, tensor.Shape{2}, []int64{1, 3})
	out := stExec(t, &Node{OpType: "Expand"}, in, shp)
	stAssertShape(t, out, tensor.Shape{2, 3})
	stAssertClose(t, out.AsFloat32(), []float32{1, 1, 1, 2, 2, 2})

	// A target dim of 1 must not shrink a larger input dim.
	in2 := stF32(t, tensor.Shape{4}, []float32{1, 2, 3, 4})
	shp2 := stI64(t, tensor.Shape{1}, []int64{1})
	out2 := stExec(t, &Node{OpType: "Expand"}, in2, shp2)
	stAssertShape(t, out2, tensor.Shape{4})
	stAssertClose(t, out2.AsFloat32(), []float32{1, 2, 3, 4})
}

// Conv1d (3D input) support: Born's base Conv is 2D-only and rejects dilation.
func TestConv1DBasic(t *testing.T) {
	x := stF32(t, tensor.Shape{1, 1, 5}, []float32{1, 2, 3, 4, 5})
	w := stF32(t, tensor.Shape{1, 1, 3}, []float32{1, 1, 1})
	out := stExec(t, &Node{OpType: "Conv"}, x, w)
	stAssertShape(t, out, tensor.Shape{1, 1, 3})
	stAssertClose(t, out.AsFloat32(), []float32{6, 9, 12})
}

func TestConv1DDilation(t *testing.T) {
	x := stF32(t, tensor.Shape{1, 1, 5}, []float32{1, 2, 3, 4, 5})
	w := stF32(t, tensor.Shape{1, 1, 3}, []float32{1, 1, 1})
	out := stExec(t, &Node{OpType: "Conv", Attributes: []Attribute{{Name: "dilations", Ints: []int64{2}}}}, x, w)
	stAssertShape(t, out, tensor.Shape{1, 1, 1})
	stAssertClose(t, out.AsFloat32(), []float32{9}) // positions 0,2,4 -> 1+3+5
}

func TestConv1DPadAndBias(t *testing.T) {
	x := stF32(t, tensor.Shape{1, 1, 3}, []float32{1, 2, 3})
	w := stF32(t, tensor.Shape{1, 1, 3}, []float32{1, 1, 1})
	b := stF32(t, tensor.Shape{1}, []float32{10})
	out := stExec(t, &Node{OpType: "Conv", Attributes: []Attribute{{Name: "pads", Ints: []int64{1, 1}}}}, x, w, b)
	stAssertShape(t, out, tensor.Shape{1, 1, 3})
	stAssertClose(t, out.AsFloat32(), []float32{13, 16, 15}) // [3,6,5]+bias 10
}

func TestConv1DDepthwise(t *testing.T) {
	x := stF32(t, tensor.Shape{1, 2, 3}, []float32{1, 2, 3, 10, 20, 30})
	w := stF32(t, tensor.Shape{2, 1, 3}, []float32{1, 1, 1, 2, 2, 2})
	out := stExec(t, &Node{OpType: "Conv", Attributes: []Attribute{{Name: "group", I: 2}}}, x, w)
	stAssertShape(t, out, tensor.Shape{1, 2, 1})
	stAssertClose(t, out.AsFloat32(), []float32{6, 120})
}

func TestElementwiseSupertonic(t *testing.T) {
	sin := stExec(t, &Node{OpType: "Sin"}, stF32(t, tensor.Shape{2}, []float32{0, float32(math.Pi / 2)}))
	stAssertClose(t, sin.AsFloat32(), []float32{0, 1})
	cos := stExec(t, &Node{OpType: "Cos"}, stF32(t, tensor.Shape{2}, []float32{0, float32(math.Pi)}))
	stAssertClose(t, cos.AsFloat32(), []float32{1, -1})
	rec := stExec(t, &Node{OpType: "Reciprocal"}, stF32(t, tensor.Shape{2}, []float32{2, 4}))
	stAssertClose(t, rec.AsFloat32(), []float32{0.5, 0.25})
	sp := stExec(t, &Node{OpType: "Softplus"}, stF32(t, tensor.Shape{1}, []float32{0}))
	stAssertClose(t, sp.AsFloat32(), []float32{0.6931472})
}

func TestTile(t *testing.T) {
	out := stExec(t, &Node{OpType: "Tile"},
		stF32(t, tensor.Shape{3}, []float32{1, 2, 3}),
		stI64(t, tensor.Shape{1}, []int64{2}))
	stAssertShape(t, out, tensor.Shape{6})
	stAssertClose(t, out.AsFloat32(), []float32{1, 2, 3, 1, 2, 3})
}

func TestReduceSum(t *testing.T) {
	out := stExec(t, &Node{OpType: "ReduceSum", Attributes: []Attribute{{Name: "keepdims", I: 1}}},
		stF32(t, tensor.Shape{2, 2}, []float32{1, 2, 3, 4}),
		stI64(t, tensor.Shape{1}, []int64{1}))
	stAssertShape(t, out, tensor.Shape{2, 1})
	stAssertClose(t, out.AsFloat32(), []float32{3, 7})
}

func TestBatchNorm(t *testing.T) {
	x := stF32(t, tensor.Shape{1, 2, 2}, []float32{1, 2, 3, 4})
	scale := stF32(t, tensor.Shape{2}, []float32{2, 2})
	bias := stF32(t, tensor.Shape{2}, []float32{1, 1})
	mean := stF32(t, tensor.Shape{2}, []float32{1, 3})
	varr := stF32(t, tensor.Shape{2}, []float32{1, 1})
	out := stExec(t, &Node{OpType: "BatchNormalization", Attributes: []Attribute{{Name: "epsilon", F: 0}}},
		x, scale, bias, mean, varr)
	stAssertShape(t, out, tensor.Shape{1, 2, 2})
	stAssertClose(t, out.AsFloat32(), []float32{1, 3, 1, 3})
}

// Pow must support int64 (integer shape arithmetic).
func TestPowInt64(t *testing.T) {
	base := stI64(t, tensor.Shape{3}, []int64{2, 3, 4})
	exp := stI64(t, tensor.Shape{1}, []int64{2})
	out := stExec(t, &Node{OpType: "Pow"}, base, exp)
	if out.DType() != tensor.Int64 {
		t.Fatalf("dtype = %s, want int64", out.DType())
	}
	got := out.AsInt64()
	want := []int64{4, 9, 16}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// Clip must support int64 (index/length clamping in the Supertonic graphs).
func TestClipInt64(t *testing.T) {
	x := stI64(t, tensor.Shape{5}, []int64{-3, 0, 5, 10, 20})
	lo := stI64(t, tensor.Shape{1}, []int64{0})
	hi := stI64(t, tensor.Shape{1}, []int64{10})
	out := stExec(t, &Node{OpType: "Clip"}, x, lo, hi)
	if out.DType() != tensor.Int64 {
		t.Fatalf("dtype = %s, want int64", out.DType())
	}
	got := out.AsInt64()
	want := []int64{0, 0, 5, 10, 10}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// refConv1D is an independent naive Conv1d oracle for property-testing the
// optimized handleConv1D paths (pointwise/im2col-matmul/direct).
func refConv1D(x, w, bias []float32, n, cin, l, cout, kL, stride, dilation, group, padBegin, padEnd int) []float32 {
	cinPerG := cin / group
	coutPerG := cout / group
	lout := (l+padBegin+padEnd-dilation*(kL-1)-1)/stride + 1
	out := make([]float32, n*cout*lout)
	for ni := 0; ni < n; ni++ {
		for oc := 0; oc < cout; oc++ {
			g := oc / coutPerG
			for ol := 0; ol < lout; ol++ {
				var sum float32
				if bias != nil {
					sum = bias[oc]
				}
				for icg := 0; icg < cinPerG; icg++ {
					ic := g*cinPerG + icg
					for k := 0; k < kL; k++ {
						il := ol*stride - padBegin + k*dilation
						if il >= 0 && il < l {
							sum += x[(ni*cin+ic)*l+il] * w[(oc*cinPerG+icg)*kL+k]
						}
					}
				}
				out[(ni*cout+oc)*lout+ol] = sum
			}
		}
	}
	return out
}

func TestConv1DMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	cases := []struct {
		name                                                     string
		n, cin, l, cout, kL, stride, dilation, group, pad0, pad1 int
		withBias                                                 bool
	}{
		{"pointwise_big", 1, 64, 40, 96, 1, 1, 1, 1, 0, 0, true},
		{"pointwise_long", 1, 24, 900, 48, 1, 1, 1, 1, 0, 0, false},
		{"k5_dil2_padded", 1, 32, 50, 64, 5, 1, 2, 1, 4, 4, true},
		{"k7_stride2", 2, 16, 37, 32, 7, 2, 1, 1, 3, 3, true},
		{"grouped", 1, 32, 30, 64, 3, 1, 1, 4, 1, 1, true},
		{"depthwise_direct", 1, 24, 33, 24, 7, 1, 1, 24, 3, 3, true},
		{"pad_exceeds_input", 1, 16, 3, 32, 7, 1, 1, 1, 6, 6, true},
		{"dil4_convnext", 1, 16, 21, 16, 5, 1, 4, 1, 8, 8, false},
		{"batch2", 2, 16, 25, 32, 3, 1, 1, 1, 1, 1, true},
		{"stride3_asym_pad", 1, 16, 41, 32, 5, 3, 1, 1, 1, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := make([]float32, tc.n*tc.cin*tc.l)
			for i := range x {
				x[i] = rng.Float32()*2 - 1
			}
			w := make([]float32, tc.cout*(tc.cin/tc.group)*tc.kL)
			for i := range w {
				w[i] = rng.Float32()*2 - 1
			}
			var bias []float32
			inputs := []*tensor.RawTensor{
				stF32(t, tensor.Shape{tc.n, tc.cin, tc.l}, x),
				stF32(t, tensor.Shape{tc.cout, tc.cin / tc.group, tc.kL}, w),
			}
			if tc.withBias {
				bias = make([]float32, tc.cout)
				for i := range bias {
					bias[i] = rng.Float32()*2 - 1
				}
				inputs = append(inputs, stF32(t, tensor.Shape{tc.cout}, bias))
			}
			node := &Node{OpType: "Conv", Attributes: []Attribute{
				{Name: "strides", Ints: []int64{int64(tc.stride)}},
				{Name: "dilations", Ints: []int64{int64(tc.dilation)}},
				{Name: "group", I: int64(tc.group)},
				{Name: "pads", Ints: []int64{int64(tc.pad0), int64(tc.pad1)}},
			}}
			got := stExec(t, node, inputs...)
			want := refConv1D(x, w, bias, tc.n, tc.cin, tc.l, tc.cout, tc.kL,
				tc.stride, tc.dilation, tc.group, tc.pad0, tc.pad1)
			lout := len(want) / (tc.n * tc.cout)
			stAssertShape(t, got, tensor.Shape{tc.n, tc.cout, lout})
			stAssertClose(t, got.AsFloat32(), want)
		})
	}
}

// Reference values computed with numpy.pad.
func TestPadConstant(t *testing.T) {
	data := stF32(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	pads := stI64(t, tensor.Shape{4}, []int64{1, 0, 1, 2}) // [b0,b1,e0,e1]
	cval := stF32(t, tensor.Shape{1}, []float32{7})
	out := stExec(t, &Node{OpType: "Pad"}, data, pads, cval)
	stAssertShape(t, out, tensor.Shape{4, 5})
	stAssertClose(t, out.AsFloat32(), []float32{
		7, 7, 7, 7, 7,
		1, 2, 3, 7, 7,
		4, 5, 6, 7, 7,
		7, 7, 7, 7, 7,
	})
}

func TestPadEdge(t *testing.T) {
	data := stF32(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	pads := stI64(t, tensor.Shape{4}, []int64{1, 1, 1, 1})
	out := stExec(t, &Node{OpType: "Pad", Attributes: []Attribute{{Name: "mode", S: []byte("edge")}}}, data, pads)
	stAssertShape(t, out, tensor.Shape{4, 5})
	stAssertClose(t, out.AsFloat32(), []float32{
		1, 1, 2, 3, 3,
		1, 1, 2, 3, 3,
		4, 4, 5, 6, 6,
		4, 4, 5, 6, 6,
	})
}

func TestPadReflect(t *testing.T) {
	data := stF32(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	pads := stI64(t, tensor.Shape{4}, []int64{0, 2, 0, 2})
	out := stExec(t, &Node{OpType: "Pad", Attributes: []Attribute{{Name: "mode", S: []byte("reflect")}}}, data, pads)
	stAssertShape(t, out, tensor.Shape{2, 7})
	stAssertClose(t, out.AsFloat32(), []float32{
		3, 2, 1, 2, 3, 2, 1,
		6, 5, 4, 5, 6, 5, 4,
	})
}
