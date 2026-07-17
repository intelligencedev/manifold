package tensor

import "testing"

// transposeSrcMap is an independent oracle: for each output position it returns
// the flat source index, computed with the original per-output-coordinate
// formulation (decompose the flat output index over newShape, recombine with
// oldStrides[axes[j]]). The production transposeData walks the output
// incrementally (odometer); a mismatch therefore points at the incremental
// index update, not at shared helpers.
func transposeSrcMap(oldShape, axes []int) []int {
	ndim := len(oldShape)
	newShape := make([]int, ndim)
	for i, ax := range axes {
		newShape[i] = oldShape[ax]
	}
	oldStrides := make([]int, ndim)
	if ndim > 0 {
		oldStrides[ndim-1] = 1
		for i := ndim - 2; i >= 0; i-- {
			oldStrides[i] = oldStrides[i+1] * oldShape[i+1]
		}
	}
	total := 1
	for _, d := range newShape {
		total *= d
	}
	srcMap := make([]int, total)
	for i := 0; i < total; i++ {
		idx := make([]int, ndim)
		tmp := i
		for j := ndim - 1; j >= 0; j-- {
			idx[j] = tmp % newShape[j]
			tmp /= newShape[j]
		}
		oldFlat := 0
		for j := 0; j < ndim; j++ {
			oldFlat += idx[j] * oldStrides[axes[j]]
		}
		srcMap[i] = oldFlat
	}
	return srcMap
}

var transposeCases = []struct {
	name string
	old  []int
	axes []int
}{
	{"1d_noop", []int{5}, []int{0}},
	{"2d_swap", []int{3, 4}, []int{1, 0}},
	{"2d_identity", []int{3, 4}, []int{0, 1}},
	{"3d_201", []int{2, 3, 4}, []int{2, 0, 1}},
	{"3d_120", []int{2, 3, 4}, []int{1, 2, 0}},
	{"3d_021", []int{2, 3, 4}, []int{0, 2, 1}},
	{"3d_210", []int{2, 3, 4}, []int{2, 1, 0}},
	{"4d_reverse", []int{2, 3, 4, 5}, []int{3, 2, 1, 0}},
	{"4d_0213", []int{2, 3, 4, 5}, []int{0, 2, 1, 3}},
	{"4d_1302", []int{2, 3, 4, 5}, []int{1, 3, 0, 2}},
	{"3d_size1", []int{1, 3, 4}, []int{2, 0, 1}}, // size-1 dim drives a carry every step
	{"4d_size1_mid", []int{2, 1, 4, 3}, []int{0, 2, 1, 3}},
	{"5d_40312", []int{2, 3, 4, 5, 6}, []int{4, 0, 3, 1, 2}}, // deep carry propagation
	{"scalar", []int{}, []int{}},                             // ndim==0, inner carry loop never runs
}

func checkTransposeF32(t *testing.T, oldShape, axes, srcMap []int) {
	t.Helper()
	x, err := NewRaw(Shape(oldShape), Float32, CPU)
	if err != nil {
		t.Fatalf("NewRaw: %v", err)
	}
	xd := x.AsFloat32()
	for k := range xd {
		xd[k] = float32(k)
	}
	out, err := TransposeAxes(x, axes...)
	if err != nil {
		t.Fatalf("TransposeAxes: %v", err)
	}
	od := out.AsFloat32()
	if len(od) != len(srcMap) {
		t.Fatalf("len got %d want %d", len(od), len(srcMap))
	}
	for i := range od {
		if od[i] != float32(srcMap[i]) {
			t.Fatalf("out[%d]=%v want %v (src %d)", i, od[i], float32(srcMap[i]), srcMap[i])
		}
	}
}

func checkTransposeF64(t *testing.T, oldShape, axes, srcMap []int) {
	t.Helper()
	x, err := NewRaw(Shape(oldShape), Float64, CPU)
	if err != nil {
		t.Fatalf("NewRaw: %v", err)
	}
	xd := x.AsFloat64()
	for k := range xd {
		xd[k] = float64(k)
	}
	out, err := TransposeAxes(x, axes...)
	if err != nil {
		t.Fatalf("TransposeAxes: %v", err)
	}
	od := out.AsFloat64()
	if len(od) != len(srcMap) {
		t.Fatalf("len got %d want %d", len(od), len(srcMap))
	}
	for i := range od {
		if od[i] != float64(srcMap[i]) {
			t.Fatalf("out[%d]=%v want %v", i, od[i], float64(srcMap[i]))
		}
	}
}

func checkTransposeI32(t *testing.T, oldShape, axes, srcMap []int) {
	t.Helper()
	x, err := NewRaw(Shape(oldShape), Int32, CPU)
	if err != nil {
		t.Fatalf("NewRaw: %v", err)
	}
	xd := x.AsInt32()
	for k := range xd {
		xd[k] = int32(k)
	}
	out, err := TransposeAxes(x, axes...)
	if err != nil {
		t.Fatalf("TransposeAxes: %v", err)
	}
	od := out.AsInt32()
	if len(od) != len(srcMap) {
		t.Fatalf("len got %d want %d", len(od), len(srcMap))
	}
	for i := range od {
		if od[i] != int32(srcMap[i]) {
			t.Fatalf("out[%d]=%v want %v", i, od[i], int32(srcMap[i]))
		}
	}
}

func checkTransposeI64(t *testing.T, oldShape, axes, srcMap []int) {
	t.Helper()
	x, err := NewRaw(Shape(oldShape), Int64, CPU)
	if err != nil {
		t.Fatalf("NewRaw: %v", err)
	}
	xd := x.AsInt64()
	for k := range xd {
		xd[k] = int64(k)
	}
	out, err := TransposeAxes(x, axes...)
	if err != nil {
		t.Fatalf("TransposeAxes: %v", err)
	}
	od := out.AsInt64()
	if len(od) != len(srcMap) {
		t.Fatalf("len got %d want %d", len(od), len(srcMap))
	}
	for i := range od {
		if od[i] != int64(srcMap[i]) {
			t.Fatalf("out[%d]=%v want %v", i, od[i], int64(srcMap[i]))
		}
	}
}

func TestTransposeAxes_IncrementalMatchesOracle(t *testing.T) {
	for _, tc := range transposeCases {
		t.Run(tc.name, func(t *testing.T) {
			srcMap := transposeSrcMap(tc.old, tc.axes)
			// x[k] = k, so each output element must equal its oracle source index.
			t.Run("f32", func(t *testing.T) { checkTransposeF32(t, tc.old, tc.axes, srcMap) })
			t.Run("f64", func(t *testing.T) { checkTransposeF64(t, tc.old, tc.axes, srcMap) })
			t.Run("i32", func(t *testing.T) { checkTransposeI32(t, tc.old, tc.axes, srcMap) })
			t.Run("i64", func(t *testing.T) { checkTransposeI64(t, tc.old, tc.axes, srcMap) })
		})
	}
}

func benchTranspose(b *testing.B, oldShape, axes []int) {
	x, err := NewRaw(Shape(oldShape), Float32, CPU)
	if err != nil {
		b.Fatal(err)
	}
	xd := x.AsFloat32()
	for k := range xd {
		xd[k] = float32(k)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := TransposeAxes(x, axes...); err != nil {
			b.Fatal(err)
		}
	}
}

// 2-D matrix transpose: write sequential, read strided.
func BenchmarkTransposeFloat32_2D(b *testing.B) {
	benchTranspose(b, []int{512, 512}, []int{1, 0})
}

// 3-D permutation similar to attention-style reshapes.
func BenchmarkTransposeFloat32_3D(b *testing.B) {
	benchTranspose(b, []int{64, 128, 64}, []int{1, 0, 2})
}
