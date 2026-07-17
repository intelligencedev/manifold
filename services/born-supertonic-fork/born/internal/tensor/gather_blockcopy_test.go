package tensor

import "testing"

// Independent oracle for ONNX Gather, kept deliberately in the original
// per-output-coordinate form (decompose the flat output index, recombine with
// the input strides). The production gather uses contiguous block copies; a
// mismatch therefore points at the block-copy logic, not at shared helpers.

func gatherProd(s []int) int {
	p := 1
	for _, d := range s {
		p *= d
	}
	return p
}

func gatherRefStrides(s []int) []int {
	st := make([]int, len(s))
	if len(s) == 0 {
		return st
	}
	st[len(s)-1] = 1
	for i := len(s) - 2; i >= 0; i-- {
		st[i] = st[i+1] * s[i+1]
	}
	return st
}

// gatherSrcMap returns, for each output position, the flat source index into x.
// Output shape is xShape[:axis] + idxShape + xShape[axis+1:].
func gatherSrcMap(xShape, idxShape, indices []int, axis int) []int {
	outShape := make([]int, 0, len(xShape)-1+len(idxShape))
	outShape = append(outShape, xShape[:axis]...)
	outShape = append(outShape, idxShape...)
	outShape = append(outShape, xShape[axis+1:]...)

	total := gatherProd(outShape)
	xStr := gatherRefStrides(xShape)
	idxStr := gatherRefStrides(idxShape)
	ndim := len(xShape)
	idxNdim := len(idxShape)

	srcMap := make([]int, total)
	for i := 0; i < total; i++ {
		outIdx := make([]int, len(outShape))
		tmp := i
		for j := len(outShape) - 1; j >= 0; j-- {
			outIdx[j] = tmp % outShape[j]
			tmp /= outShape[j]
		}
		idxFlat := 0
		for j := 0; j < idxNdim; j++ {
			idxFlat += outIdx[axis+j] * idxStr[j]
		}
		g := indices[idxFlat]
		srcFlat := 0
		for j := 0; j < axis; j++ {
			srcFlat += outIdx[j] * xStr[j]
		}
		srcFlat += g * xStr[axis]
		for j := axis + 1; j < ndim; j++ {
			srcFlat += outIdx[axis+idxNdim+j-axis-1] * xStr[j]
		}
		srcMap[i] = srcFlat
	}
	return srcMap
}

var gatherCases = []struct {
	name string
	x    []int
	idx  []int
	axis int
}{
	{"1d_axis0", []int{5}, []int{3}, 0},
	{"1d_scalar_idx", []int{5}, []int{}, 0}, // scalar index -> scalar output
	{"2d_axis0", []int{4, 3}, []int{2}, 0},
	{"2d_axis1_post1", []int{4, 3}, []int{2}, 1}, // axis is last -> post size 1
	{"3d_axis1_1didx", []int{2, 5, 4}, []int{3}, 1},
	{"3d_axis1_2didx", []int{2, 5, 4}, []int{3, 2}, 1},
	{"3d_axislast", []int{2, 3, 4}, []int{5}, 2},
	{"3d_axis0_2didx", []int{4, 3, 2}, []int{2, 3}, 0},
	{"4d_axis2", []int{2, 3, 4, 2}, []int{3}, 2},
	{"4d_axis1_2didx", []int{2, 3, 4, 2}, []int{2, 2}, 1}, // pre>1, multi-dim idx, post>1, 4-D
}

// deterministicIndices fills idxN values in [0, axisDim).
func deterministicIndices(idxN, axisDim int) []int {
	v := make([]int, idxN)
	for k := range v {
		v[k] = (k*7 + 3) % axisDim
	}
	return v
}

func makeInt32Index(t *testing.T, idxShape, idxVals []int) *RawTensor {
	t.Helper()
	idxT, err := NewRaw(Shape(idxShape), Int32, CPU)
	if err != nil {
		t.Fatalf("idx NewRaw: %v", err)
	}
	id := idxT.AsInt32()
	for k := range id {
		id[k] = int32(idxVals[k])
	}
	return idxT
}

// Each checkGather* helper fills x with x[k]=k for its dtype, gathers, and
// asserts every output element equals its oracle source index value.

func checkGatherF32(t *testing.T, xShape []int, axis int, idxT *RawTensor, srcMap []int) {
	t.Helper()
	x, err := NewRaw(Shape(xShape), Float32, CPU)
	if err != nil {
		t.Fatalf("x NewRaw: %v", err)
	}
	xd := x.AsFloat32()
	for k := range xd {
		xd[k] = float32(k)
	}
	out, err := Gather(x, idxT, axis)
	if err != nil {
		t.Fatalf("Gather: %v", err)
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

func checkGatherF64(t *testing.T, xShape []int, axis int, idxT *RawTensor, srcMap []int) {
	t.Helper()
	x, err := NewRaw(Shape(xShape), Float64, CPU)
	if err != nil {
		t.Fatalf("x NewRaw: %v", err)
	}
	xd := x.AsFloat64()
	for k := range xd {
		xd[k] = float64(k)
	}
	out, err := Gather(x, idxT, axis)
	if err != nil {
		t.Fatalf("Gather: %v", err)
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

func checkGatherI32(t *testing.T, xShape []int, axis int, idxT *RawTensor, srcMap []int) {
	t.Helper()
	x, err := NewRaw(Shape(xShape), Int32, CPU)
	if err != nil {
		t.Fatalf("x NewRaw: %v", err)
	}
	xd := x.AsInt32()
	for k := range xd {
		xd[k] = int32(k)
	}
	out, err := Gather(x, idxT, axis)
	if err != nil {
		t.Fatalf("Gather: %v", err)
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

func checkGatherI64(t *testing.T, xShape []int, axis int, idxT *RawTensor, srcMap []int) {
	t.Helper()
	x, err := NewRaw(Shape(xShape), Int64, CPU)
	if err != nil {
		t.Fatalf("x NewRaw: %v", err)
	}
	xd := x.AsInt64()
	for k := range xd {
		xd[k] = int64(k)
	}
	out, err := Gather(x, idxT, axis)
	if err != nil {
		t.Fatalf("Gather: %v", err)
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

func TestGather_BlockCopyMatchesOracle(t *testing.T) {
	for _, tc := range gatherCases {
		t.Run(tc.name, func(t *testing.T) {
			axisDim := tc.x[tc.axis]
			idxN := gatherProd(tc.idx)
			idxVals := deterministicIndices(idxN, axisDim)
			srcMap := gatherSrcMap(tc.x, tc.idx, idxVals, tc.axis)
			idxT := makeInt32Index(t, tc.idx, idxVals)

			// x[k] = k, so each output element must equal its oracle source index.
			t.Run("f32", func(t *testing.T) { checkGatherF32(t, tc.x, tc.axis, idxT, srcMap) })
			t.Run("f64", func(t *testing.T) { checkGatherF64(t, tc.x, tc.axis, idxT, srcMap) })
			t.Run("i32", func(t *testing.T) { checkGatherI32(t, tc.x, tc.axis, idxT, srcMap) })
			t.Run("i64", func(t *testing.T) { checkGatherI64(t, tc.x, tc.axis, idxT, srcMap) })
		})
	}
}

// TestGather_NegativeIndexAndAxis verifies ONNX negative-index normalization
// (i -> axis_size + i) and negative-axis handling: gathering with negative
// indices/axis must equal gathering with the equivalent non-negative ones.
func TestGather_NegativeIndexAndAxis(t *testing.T) {
	xShape := []int{4, 3}

	// axis 0 (size 4): negative indices normalize to the tail rows.
	rawIdx := []int{-1, -4, 0, 3}
	normIdx := []int{3, 0, 0, 3}
	idxT := makeInt32Index(t, []int{4}, rawIdx)
	srcMap := gatherSrcMap(xShape, []int{4}, normIdx, 0)
	t.Run("neg_index_axis0", func(t *testing.T) { checkGatherF32(t, xShape, 0, idxT, srcMap) })

	// Negative axis (-1 == axis 1, size 3) combined with negative indices.
	rawIdx2 := []int{-1, 0, -3}
	normIdx2 := []int{2, 0, 0}
	idxT2 := makeInt32Index(t, []int{3}, rawIdx2)
	srcMap2 := gatherSrcMap(xShape, []int{3}, normIdx2, 1)
	t.Run("neg_index_neg_axis", func(t *testing.T) { checkGatherF32(t, xShape, -1, idxT2, srcMap2) })
}

func benchGather(b *testing.B, xShape, idxShape []int, axis int) {
	axisDim := xShape[axis]
	x, err := NewRaw(Shape(xShape), Float32, CPU)
	if err != nil {
		b.Fatal(err)
	}
	xd := x.AsFloat32()
	for k := range xd {
		xd[k] = float32(k)
	}
	idxT, err := NewRaw(Shape(idxShape), Int32, CPU)
	if err != nil {
		b.Fatal(err)
	}
	id := idxT.AsInt32()
	for k := range id {
		id[k] = int32((k*7 + 3) % axisDim)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Gather(x, idxT, axis); err != nil {
			b.Fatal(err)
		}
	}
}

// Gather on the leading axis: each gathered row is a contiguous post-sized block.
func BenchmarkGatherFloat32_Axis0(b *testing.B) {
	benchGather(b, []int{256, 512}, []int{256}, 0)
}

// Gather on the last axis: post size 1 (worst case for block copy).
func BenchmarkGatherFloat32_AxisLast(b *testing.B) {
	benchGather(b, []int{512, 256}, []int{256}, 1)
}
