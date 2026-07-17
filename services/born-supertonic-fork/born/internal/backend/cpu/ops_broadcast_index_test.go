package cpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// refFlatIndex is an independent, closed-form oracle for the broadcast source
// index of output element outIdx. It decomposes outIdx into per-dimension
// coordinates using the output strides and recombines them with the
// broadcast-adjusted input strides. The production broadcast ops compute the
// same index incrementally (odometer); this oracle deliberately uses the
// straightforward division form so a mismatch points at the incremental logic.
func refFlatIndex(outIdx int, outStrides, inStrides []int) int {
	idx := 0
	for d := range outStrides {
		coord := outIdx / outStrides[d]
		outIdx %= outStrides[d]
		idx += coord * inStrides[d]
	}
	return idx
}

// broadcastCase is a pair of input shapes that broadcast to outShape.
type broadcastCase struct {
	name     string
	aShape   tensor.Shape
	bShape   tensor.Shape
	outShape tensor.Shape
}

// broadcastCases exercises the carry logic across many stride patterns:
// no broadcast, leading-1, trailing-1, interior-1, mixed, and 4-D.
var broadcastCases = []broadcastCase{
	{"equal_1d", tensor.Shape{5}, tensor.Shape{5}, tensor.Shape{5}},
	{"equal_2d", tensor.Shape{2, 3}, tensor.Shape{2, 3}, tensor.Shape{2, 3}},
	{"scalar_vs_vec", tensor.Shape{1}, tensor.Shape{5}, tensor.Shape{5}},
	{"vec_vs_scalar", tensor.Shape{5}, tensor.Shape{1}, tensor.Shape{5}},
	{"col_vs_row", tensor.Shape{3, 1}, tensor.Shape{1, 4}, tensor.Shape{3, 4}},
	{"row_vs_col", tensor.Shape{1, 4}, tensor.Shape{3, 1}, tensor.Shape{3, 4}},
	{"mat_vs_row", tensor.Shape{3, 4}, tensor.Shape{1, 4}, tensor.Shape{3, 4}},
	{"3d_interior1", tensor.Shape{2, 1, 4}, tensor.Shape{1, 3, 1}, tensor.Shape{2, 3, 4}},
	{"3d_full_vs_111", tensor.Shape{2, 3, 4}, tensor.Shape{1, 1, 1}, tensor.Shape{2, 3, 4}},
	{"3d_prefix", tensor.Shape{1, 3, 4}, tensor.Shape{2, 1, 1}, tensor.Shape{2, 3, 4}},
	{"4d_mixed", tensor.Shape{2, 1, 3, 1}, tensor.Shape{1, 4, 1, 5}, tensor.Shape{2, 4, 3, 5}},
	{"5d_mixed", tensor.Shape{2, 1, 3, 1, 5}, tensor.Shape{1, 4, 1, 6, 1}, tensor.Shape{2, 4, 3, 6, 5}},
	// Degenerate shapes. These cannot reach the broadcast ops through the public
	// backend (shape validation rejects 0 dims, and scalar+scalar takes the
	// non-broadcast fast path), but they pin the odometer's edge paths: scalar
	// (ndim==0, the inner carry loop never runs) and empty output (n==0, the
	// element loop never runs).
	{"scalar", tensor.Shape{}, tensor.Shape{}, tensor.Shape{}},
	{"empty_out", tensor.Shape{0, 4}, tensor.Shape{1, 4}, tensor.Shape{0, 4}},
}

// fillSeqF32 and fillSeqF64 fill s with the deterministic ramp step*i + offset.
// Callers give a and b linearly-independent steps so that an index error which
// shifts both source reads by the same amount changes the result value (and is
// caught) even for the subtraction op, instead of relying on an out-of-bounds
// panic. The offset keeps values away from zero so division never divides by zero.
func fillSeqF32(s []float32, step, offset float32) {
	for i := range s {
		s[i] = float32(i)*step + offset
	}
}

func fillSeqF64(s []float64, step, offset float64) {
	for i := range s {
		s[i] = float64(i)*step + offset
	}
}

func TestBroadcastFloat32_MatchesOracle(t *testing.T) {
	ops := []struct {
		name string
		fn   func(dst, a, b []float32, aShape, bShape, outShape tensor.Shape)
		op   func(x, y float32) float32
	}{
		{"add", addBroadcastFloat32, func(x, y float32) float32 { return x + y }},
		{"sub", subBroadcastFloat32, func(x, y float32) float32 { return x - y }},
		{"mul", mulBroadcastFloat32, func(x, y float32) float32 { return x * y }},
		{"div", divBroadcastFloat32, func(x, y float32) float32 { return x / y }},
	}
	for _, op := range ops {
		for _, tc := range broadcastCases {
			t.Run(op.name+"/"+tc.name, func(t *testing.T) {
				a := make([]float32, tc.aShape.NumElements())
				b := make([]float32, tc.bShape.NumElements())
				fillSeqF32(a, 1, 1)
				fillSeqF32(b, 2, 2) // distinct slope; offset 2 -> never zero, safe for div

				n := tc.outShape.NumElements()
				outStrides := tc.outShape.ComputeStrides()
				aStrides := computeBroadcastStridesForShape(tc.aShape, tc.outShape)
				bStrides := computeBroadcastStridesForShape(tc.bShape, tc.outShape)

				want := make([]float32, n)
				for i := 0; i < n; i++ {
					ai := refFlatIndex(i, outStrides, aStrides)
					bi := refFlatIndex(i, outStrides, bStrides)
					want[i] = op.op(a[ai], b[bi])
				}

				got := make([]float32, n)
				op.fn(got, a, b, tc.aShape, tc.bShape, tc.outShape)

				for i := 0; i < n; i++ {
					if got[i] != want[i] { // bit-exact: same operands, same order
						t.Fatalf("%s %s: got[%d]=%v want %v", op.name, tc.name, i, got[i], want[i])
					}
				}
			})
		}
	}
}

func TestBroadcastFloat64_MatchesOracle(t *testing.T) {
	ops := []struct {
		name string
		fn   func(dst, a, b []float64, aShape, bShape, outShape tensor.Shape)
		op   func(x, y float64) float64
	}{
		{"add", addBroadcastFloat64, func(x, y float64) float64 { return x + y }},
		{"sub", subBroadcastFloat64, func(x, y float64) float64 { return x - y }},
		{"mul", mulBroadcastFloat64, func(x, y float64) float64 { return x * y }},
		{"div", divBroadcastFloat64, func(x, y float64) float64 { return x / y }},
	}
	for _, op := range ops {
		for _, tc := range broadcastCases {
			t.Run(op.name+"/"+tc.name, func(t *testing.T) {
				a := make([]float64, tc.aShape.NumElements())
				b := make([]float64, tc.bShape.NumElements())
				fillSeqF64(a, 1, 1)
				fillSeqF64(b, 2, 2) // distinct slope; offset 2 -> never zero, safe for div

				n := tc.outShape.NumElements()
				outStrides := tc.outShape.ComputeStrides()
				aStrides := computeBroadcastStridesForShape(tc.aShape, tc.outShape)
				bStrides := computeBroadcastStridesForShape(tc.bShape, tc.outShape)

				want := make([]float64, n)
				for i := 0; i < n; i++ {
					ai := refFlatIndex(i, outStrides, aStrides)
					bi := refFlatIndex(i, outStrides, bStrides)
					want[i] = op.op(a[ai], b[bi])
				}

				got := make([]float64, n)
				op.fn(got, a, b, tc.aShape, tc.bShape, tc.outShape)

				for i := 0; i < n; i++ {
					if got[i] != want[i] {
						t.Fatalf("%s %s: got[%d]=%v want %v", op.name, tc.name, i, got[i], want[i])
					}
				}
			})
		}
	}
}

// benchBroadcastShape mirrors a representative elementwise broadcast in the
// BirdNET forward pass: a full tensor scaled by a row-broadcast operand.
var benchAShape = tensor.Shape{256, 512}
var benchBShape = tensor.Shape{1, 512}
var benchOutShape = tensor.Shape{256, 512}

func BenchmarkMulBroadcastFloat32(b *testing.B) {
	a := make([]float32, benchAShape.NumElements())
	bb := make([]float32, benchBShape.NumElements())
	dst := make([]float32, benchOutShape.NumElements())
	fillSeqF32(a, 1, 1)
	fillSeqF32(bb, 2, 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mulBroadcastFloat32(dst, a, bb, benchAShape, benchBShape, benchOutShape)
	}
}

func BenchmarkAddBroadcastFloat32(b *testing.B) {
	a := make([]float32, benchAShape.NumElements())
	bb := make([]float32, benchBShape.NumElements())
	dst := make([]float32, benchOutShape.NumElements())
	fillSeqF32(a, 1, 1)
	fillSeqF32(bb, 2, 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		addBroadcastFloat32(dst, a, bb, benchAShape, benchBShape, benchOutShape)
	}
}

func BenchmarkSubBroadcastFloat32(b *testing.B) {
	a := make([]float32, benchAShape.NumElements())
	bb := make([]float32, benchBShape.NumElements())
	dst := make([]float32, benchOutShape.NumElements())
	fillSeqF32(a, 1, 1)
	fillSeqF32(bb, 2, 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		subBroadcastFloat32(dst, a, bb, benchAShape, benchBShape, benchOutShape)
	}
}

func BenchmarkDivBroadcastFloat32(b *testing.B) {
	a := make([]float32, benchAShape.NumElements())
	bb := make([]float32, benchBShape.NumElements())
	dst := make([]float32, benchOutShape.NumElements())
	fillSeqF32(a, 1, 1)
	fillSeqF32(bb, 2, 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		divBroadcastFloat32(dst, a, bb, benchAShape, benchBShape, benchOutShape)
	}
}
