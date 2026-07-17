//go:build windows || linux

package webgpu

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// newFloat32Raw creates a CPU float32 RawTensor from a flat slice.
func newFloat32Raw(t *testing.T, shape tensor.Shape, data []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw(%v, Float32): %v", shape, err)
	}
	copy(raw.AsFloat32(), data)
	return raw
}

// newInt32Raw creates a CPU int32 RawTensor from a flat slice.
func newInt32Raw(t *testing.T, shape tensor.Shape, data []int32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Int32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw(%v, Int32): %v", shape, err)
	}
	copy(raw.AsInt32(), data)
	return raw
}

const scatterTol float32 = 1e-5

// approxEqualF32 reports whether |a-b| <= scatterTol.
func approxEqualF32(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= scatterTol
}

// =============================================================================
// SelectAdd GPU lazy path tests
// =============================================================================

// TestSelectAdd_GPU_UniqueIndices verifies that selectAddShader correctly
// copies dest and accumulates distinct src rows into their target rows.
//
// dest [3,2]:  [[1,2],[3,4],[5,6]]
// indices [2]: [0, 2]
// src [2,2]:   [[10,20],[30,40]]
// want [3,2]:  [[11,22],[3,4],[35,46]].
func TestSelectAdd_GPU_UniqueIndices(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	dest := newFloat32Raw(t, tensor.Shape{3, 2}, []float32{1, 2, 3, 4, 5, 6})
	idx := newInt32Raw(t, tensor.Shape{2}, []int32{0, 2})
	src := newFloat32Raw(t, tensor.Shape{2, 2}, []float32{10, 20, 30, 40})

	got := b.SelectAdd(dest, 0, idx, src)

	want := []float32{11, 22, 3, 4, 35, 46}
	gotData := got.AsFloat32()
	for i, w := range want {
		if !approxEqualF32(gotData[i], w) {
			t.Errorf("SelectAdd GPU unique: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestSelectAdd_GPU_DuplicateIndices verifies accumulation when two src rows
// target the same dest row (the key embedding-backward case).
//
// dest [2,3]:  [[0,0,0],[0,0,0]]
// indices [3]: [0, 1, 0]   — row 0 receives rows 0 and 2 of src
// src [3,3]:   [[1,2,3],[4,5,6],[7,8,9]]
// want [2,3]:  [[8,10,12],[4,5,6]].
func TestSelectAdd_GPU_DuplicateIndices(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	dest := newFloat32Raw(t, tensor.Shape{2, 3}, []float32{0, 0, 0, 0, 0, 0})
	idx := newInt32Raw(t, tensor.Shape{3}, []int32{0, 1, 0})
	src := newFloat32Raw(t, tensor.Shape{3, 3}, []float32{1, 2, 3, 4, 5, 6, 7, 8, 9})

	got := b.SelectAdd(dest, 0, idx, src)

	want := []float32{8, 10, 12, 4, 5, 6}
	gotData := got.AsFloat32()
	for i, w := range want {
		if !approxEqualF32(gotData[i], w) {
			t.Errorf("SelectAdd GPU duplicate: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestSelectAdd_GPU_NonZeroDest verifies that dest values are preserved and
// accumulated onto (not overwritten).
//
// dest [2,2]:  [[10,20],[30,40]]
// indices [1]: [1]
// src [1,2]:   [[1,2]]
// want [2,2]:  [[10,20],[31,42]].
func TestSelectAdd_GPU_NonZeroDest(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	dest := newFloat32Raw(t, tensor.Shape{2, 2}, []float32{10, 20, 30, 40})
	idx := newInt32Raw(t, tensor.Shape{1}, []int32{1})
	src := newFloat32Raw(t, tensor.Shape{1, 2}, []float32{1, 2})

	got := b.SelectAdd(dest, 0, idx, src)

	want := []float32{10, 20, 31, 42}
	gotData := got.AsFloat32()
	for i, w := range want {
		if !approxEqualF32(gotData[i], w) {
			t.Errorf("SelectAdd GPU non-zero dest: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestSelectAdd_GPU_ResultIsLazy verifies the GPU path returns a lazy tensor
// that becomes realized only on first Data() access.
func TestSelectAdd_GPU_ResultIsLazy(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	if !b.LazyMode {
		t.Skip("LazyMode is off; GPU lazy path not exercised")
	}

	dest := newFloat32Raw(t, tensor.Shape{4, 2}, []float32{0, 0, 0, 0, 0, 0, 0, 0})
	idx := newInt32Raw(t, tensor.Shape{2}, []int32{1, 3})
	src := newFloat32Raw(t, tensor.Shape{2, 2}, []float32{1, 2, 3, 4})

	result := b.SelectAdd(dest, 0, idx, src)

	if !result.IsLazy() {
		t.Error("SelectAdd GPU: result should be lazy before Data() is called")
	}

	_ = result.AsFloat32() // triggers realization

	if result.IsLazy() {
		t.Error("SelectAdd GPU: result should be realized after AsFloat32()")
	}
}

// TestSelectAdd_GPU_OriginalDestUnmodified verifies dest is not mutated.
func TestSelectAdd_GPU_OriginalDestUnmodified(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	orig := []float32{1, 2, 3, 4}
	dest := newFloat32Raw(t, tensor.Shape{2, 2}, orig)
	idx := newInt32Raw(t, tensor.Shape{1}, []int32{0})
	src := newFloat32Raw(t, tensor.Shape{1, 2}, []float32{9, 9})

	_ = b.SelectAdd(dest, 0, idx, src)

	destData := dest.AsFloat32()
	for i, v := range orig {
		if destData[i] != v {
			t.Errorf("SelectAdd GPU: dest[%d] was modified: got %v, want %v", i, destData[i], v)
		}
	}
}

// =============================================================================
// ScatterAdd GPU lazy path tests
// =============================================================================

// TestScatterAdd_GPU_Basic tests the fundamental scatter-add semantics for
// a 3-D tensor [batch=1, seq=3, hidden=2] scattering along dim=1.
//
// dest [1,5,2]:   all zeros
// indices [1,3,2]: [[[0,1],[2,3],[4,4]]]  — for each src element, which dest row along dim 1
// src [1,3,2]:     [[[1,2],[3,4],[5,6]]]
//
// Expected result [1,5,2]:
//
//	row 0: [1,0]   (src[0,0,0]=1 → dest[0,0,0]; src[0,0,1]=2 BUT index=1 → dest[0,1,1])
//	row 1: [0,2]
//	row 2: [3,0]
//	row 3: [0,4]
//	row 4: [5,6]   (both src[0,2,:] point here)
//
// We use a simpler concrete example to avoid index-shape confusion.
func TestScatterAdd_GPU_Basic(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	// dest [3]: [0, 0, 0]  (1-D for simplicity, scatter dim=0)
	dest := newFloat32Raw(t, tensor.Shape{3}, []float32{0, 0, 0})
	// src [2]:     [10, 20]
	src := newFloat32Raw(t, tensor.Shape{2}, []float32{10, 20})
	// indices [2]: [1, 0] — src[0]=10 → dest[1], src[1]=20 → dest[0]
	idx := newInt32Raw(t, tensor.Shape{2}, []int32{1, 0})

	got := b.ScatterAdd(dest, 0, idx, src)

	want := []float32{20, 10, 0}
	gotData := got.AsFloat32()
	for i, w := range want {
		if !approxEqualF32(gotData[i], w) {
			t.Errorf("ScatterAdd GPU basic: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestScatterAdd_GPU_AccumulatesDuplicates verifies that two src elements
// scattering to the same dest position are summed.
//
// dest [3]: [0, 0, 0]
// src [3]:  [1, 2, 3]
// idx [3]:  [0, 0, 2]  — both 0 and 1 go to dest[0]
// want [3]: [3, 0, 3].
func TestScatterAdd_GPU_AccumulatesDuplicates(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	dest := newFloat32Raw(t, tensor.Shape{3}, []float32{0, 0, 0})
	src := newFloat32Raw(t, tensor.Shape{3}, []float32{1, 2, 3})
	idx := newInt32Raw(t, tensor.Shape{3}, []int32{0, 0, 2})

	got := b.ScatterAdd(dest, 0, idx, src)

	want := []float32{3, 0, 3}
	gotData := got.AsFloat32()
	for i, w := range want {
		if !approxEqualF32(gotData[i], w) {
			t.Errorf("ScatterAdd GPU duplicates: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestScatterAdd_GPU_PreservesDestValues verifies pre-existing dest values
// are not zeroed out and accumulation is additive.
func TestScatterAdd_GPU_PreservesDestValues(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	// dest [3]: [100, 200, 300]
	// src scatters 50 into position 1 only
	dest := newFloat32Raw(t, tensor.Shape{3}, []float32{100, 200, 300})
	src := newFloat32Raw(t, tensor.Shape{1}, []float32{50})
	idx := newInt32Raw(t, tensor.Shape{1}, []int32{1})

	got := b.ScatterAdd(dest, 0, idx, src)

	want := []float32{100, 250, 300}
	gotData := got.AsFloat32()
	for i, w := range want {
		if !approxEqualF32(gotData[i], w) {
			t.Errorf("ScatterAdd GPU preserve: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestScatterAdd_GPU_2D_Dim1 covers the 2-D [batch, seq] → scatter along dim=1 case
// that arises in the Gather backward for attention weight gradients.
//
// dest [2,4]:  all zeros
// src [2,2]:   [[1,2],[3,4]]
// idx [2,2]:   [[3,0],[1,2]]
// want [2,4]:  row0=[2,0,0,1]  row1=[0,3,4,0].
func TestScatterAdd_GPU_2D_Dim1(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	dest := newFloat32Raw(t, tensor.Shape{2, 4}, []float32{0, 0, 0, 0, 0, 0, 0, 0})
	src := newFloat32Raw(t, tensor.Shape{2, 2}, []float32{1, 2, 3, 4})
	idx := newInt32Raw(t, tensor.Shape{2, 2}, []int32{3, 0, 1, 2})

	got := b.ScatterAdd(dest, 1, idx, src)

	// src[0,0]=1 → dest[0, idx[0,0]=3], src[0,1]=2 → dest[0, idx[0,1]=0]
	// src[1,0]=3 → dest[1, idx[1,0]=1], src[1,1]=4 → dest[1, idx[1,1]=2]
	want := []float32{2, 0, 0, 1, 0, 3, 4, 0}
	gotData := got.AsFloat32()
	for i, w := range want {
		if !approxEqualF32(gotData[i], w) {
			t.Errorf("ScatterAdd GPU 2D dim1: got[%d]=%v, want=%v", i, gotData[i], w)
		}
	}
}

// TestScatterAdd_GPU_ResultIsLazy verifies the GPU path returns a lazy tensor.
func TestScatterAdd_GPU_ResultIsLazy(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	if !b.LazyMode {
		t.Skip("LazyMode is off; GPU lazy path not exercised")
	}

	dest := newFloat32Raw(t, tensor.Shape{4}, []float32{0, 0, 0, 0})
	src := newFloat32Raw(t, tensor.Shape{2}, []float32{1, 2})
	idx := newInt32Raw(t, tensor.Shape{2}, []int32{0, 3})

	result := b.ScatterAdd(dest, 0, idx, src)

	if !result.IsLazy() {
		t.Error("ScatterAdd GPU: result should be lazy before Data() is called")
	}

	_ = result.AsFloat32()

	if result.IsLazy() {
		t.Error("ScatterAdd GPU: result should be realized after AsFloat32()")
	}
}

// TestScatterAdd_GPU_OriginalDestUnmodified verifies ScatterAdd does not mutate dest.
func TestScatterAdd_GPU_OriginalDestUnmodified(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	b, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer b.Release()

	orig := []float32{7, 8, 9}
	dest := newFloat32Raw(t, tensor.Shape{3}, orig)
	src := newFloat32Raw(t, tensor.Shape{1}, []float32{99})
	idx := newInt32Raw(t, tensor.Shape{1}, []int32{0})

	_ = b.ScatterAdd(dest, 0, idx, src)

	destData := dest.AsFloat32()
	for i, v := range orig {
		if destData[i] != v {
			t.Errorf("ScatterAdd GPU: dest[%d] was modified: got %v, want %v", i, destData[i], v)
		}
	}
}

// =============================================================================
// GPU vs CPU parity test — ensures GPU and CPU produce identical results
// =============================================================================

// TestSelectAdd_GPU_CPU_Parity runs SelectAdd on a shared input through both
// the CPU backend and the WebGPU lazy backend and asserts numeric agreement.
func TestSelectAdd_GPU_CPU_Parity(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	gpu, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer gpu.Release()

	const (
		numRows    = 8
		innerSize  = 16
		numIndices = 12
	)

	// Build dest: ascending values.
	destData := make([]float32, numRows*innerSize)
	for i := range destData {
		destData[i] = float32(i) * 0.1
	}

	// Build src: descending values.
	srcData := make([]float32, numIndices*innerSize)
	for i := range srcData {
		srcData[i] = float32(numIndices*innerSize-i) * 0.05
	}

	// Build indices: cyclic pattern to test duplicate accumulation.
	idxData := make([]int32, numIndices)
	for i := range idxData {
		idxData[i] = int32(i % numRows)
	}

	dest := newFloat32Raw(t, tensor.Shape{numRows, innerSize}, destData)
	src := newFloat32Raw(t, tensor.Shape{numIndices, innerSize}, srcData)
	idx := newInt32Raw(t, tensor.Shape{numIndices}, idxData)

	gpuResult := gpu.SelectAdd(dest, 0, idx, src)
	gpuData := gpuResult.AsFloat32()

	// CPU reference via the CPU fallback.
	cpuResult := gpu.selectAddCPU(dest, 0, idx, src, dest.Shape(), src.Shape(), numIndices)
	cpuData := cpuResult.AsFloat32()

	if len(gpuData) != len(cpuData) {
		t.Fatalf("SelectAdd parity: length mismatch: gpu=%d cpu=%d", len(gpuData), len(cpuData))
	}
	for i := range cpuData {
		if math.Abs(float64(gpuData[i]-cpuData[i])) > 1e-4 {
			t.Errorf("SelectAdd parity: index %d: gpu=%v cpu=%v diff=%v",
				i, gpuData[i], cpuData[i], gpuData[i]-cpuData[i])
		}
	}
}

// TestScatterAdd_GPU_CPU_Parity runs ScatterAdd on identical inputs through both
// the CPU fallback and the WebGPU lazy path and asserts numeric agreement.
func TestScatterAdd_GPU_CPU_Parity(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU compute not available")
	}
	gpu, err := New()
	if err != nil {
		t.Skipf("WebGPU not available: %v", err)
	}
	defer gpu.Release()

	const (
		batch  = 2
		srcSeq = 3
		dstSeq = 5
	)

	// dest [batch, dstSeq]: zeros.
	dest := newFloat32Raw(t, tensor.Shape{batch, dstSeq}, make([]float32, batch*dstSeq))

	// src [batch, srcSeq]: simple ascending values.
	srcData := make([]float32, batch*srcSeq)
	for i := range srcData {
		srcData[i] = float32(i + 1)
	}
	src := newFloat32Raw(t, tensor.Shape{batch, srcSeq}, srcData)

	// indices [batch, srcSeq]: constrained to [0, dstSeq).
	idxData := []int32{2, 0, 4, 1, 3, 2}
	idx := newInt32Raw(t, tensor.Shape{batch, srcSeq}, idxData)

	gpuResult := gpu.ScatterAdd(dest, 1, idx, src)
	gpuData := gpuResult.AsFloat32()

	cpuResult := gpu.scatterAddCPU(dest, 1, idx, src)
	cpuData := cpuResult.AsFloat32()

	if len(gpuData) != len(cpuData) {
		t.Fatalf("ScatterAdd parity: length mismatch: gpu=%d cpu=%d", len(gpuData), len(cpuData))
	}
	for i := range cpuData {
		if math.Abs(float64(gpuData[i]-cpuData[i])) > 1e-4 {
			t.Errorf("ScatterAdd parity: index %d: gpu=%v cpu=%v diff=%v",
				i, gpuData[i], cpuData[i], gpuData[i]-cpuData[i])
		}
	}
}
