//go:build !wasm

package operators

import (
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// naiveDepthwise is an independent reference: explicit-index depthwise conv with
// the same (kh, kw) accumulation order as the production kernel, so a correct
// kernel matches it bit-for-bit while an indexing or stride bug diverges.
func naiveDepthwise(in, weight []float32, n, c, hp, wp, kh, kw, hOut, wOut, s int) []float32 {
	out := make([]float32, n*c*hOut*wOut)
	for plane := 0; plane < n*c; plane++ {
		ci := plane % c
		for oh := 0; oh < hOut; oh++ {
			for ow := 0; ow < wOut; ow++ {
				var sum float32
				for r := 0; r < kh; r++ {
					for q := 0; q < kw; q++ {
						sum += in[(plane*hp+oh*s+r)*wp+ow*s+q] * weight[(ci*kh+r)*kw+q]
					}
				}
				out[(plane*hOut+oh)*wOut+ow] = sum
			}
		}
	}
	return out
}

type dwCase struct {
	name             string
	n, c, h, w, k, s int
}

var depthwiseCases = []dwCase{
	{"3x3_s1", 1, 4, 8, 8, 3, 1},
	{"3x3_s2", 1, 8, 16, 16, 3, 2},
	{"3x3_s1_multibatch", 2, 3, 7, 9, 3, 1},
	{"1x1_s1", 1, 5, 6, 6, 1, 1},
	{"5x5_s1", 1, 6, 11, 11, 5, 1},
	{"3x3_s2_odd", 1, 4, 9, 7, 3, 2},
	{"3x3_s1_wide", 1, 64, 12, 12, 3, 1},
}

func fillDet(s []float32) {
	for i := range s {
		s[i] = float32((i*37+11)%97)*0.1 - 4.0
	}
}

// TestDepthwiseConvForward_MatchesNaive checks the kernel arithmetic against an
// independent explicit-index reference, bit-for-bit (same accumulation order).
func TestDepthwiseConvForward_MatchesNaive(t *testing.T) {
	// Force the scalar reference path: this test asserts bit-for-bit equality with
	// the explicit-index reference, which only holds for the scalar kernel's
	// accumulation order. The vendored SIMD kernel reorders FMA accumulation and is
	// covered within tolerance by TestDepthwise3x3SIMDParity.
	defer func(prev func(out, in, weight []float32, n, c, hp, wp, hOut, wOut int)) {
		depthwise3x3F32 = prev
	}(depthwise3x3F32)
	depthwise3x3F32 = nil

	for _, tc := range depthwiseCases {
		t.Run(tc.name, func(t *testing.T) {
			hOut := (tc.h-tc.k)/tc.s + 1
			wOut := (tc.w-tc.k)/tc.s + 1
			in := make([]float32, tc.n*tc.c*tc.h*tc.w)
			weight := make([]float32, tc.c*tc.k*tc.k)
			fillDet(in)
			fillDet(weight)

			want := naiveDepthwise(in, weight, tc.n, tc.c, tc.h, tc.w, tc.k, tc.k, hOut, wOut, tc.s)
			got := make([]float32, len(want))
			depthwiseConvForwardFloat32(got, in, weight, tc.n, tc.c, tc.h, tc.w, tc.k, tc.k, hOut, wOut, tc.s)

			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("idx %d: got %v want %v", i, got[i], want[i])
				}
			}
		})
	}
}

// TestDepthwise_MatchesGroupedConv proves the fast path produces the same result
// as the existing groupedConv2D (im2col + per-channel GEMM) it replaces, within
// the model's parity tolerance (the two sum in different orders).
func checkDepthwiseVsGrouped(t *testing.T, ctx *Context, tc dwCase) {
	t.Helper()
	in, err := tensor.NewRaw(tensor.Shape{tc.n, tc.c, tc.h, tc.w}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	w, err := tensor.NewRaw(tensor.Shape{tc.c, 1, tc.k, tc.k}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	fillDet(in.AsFloat32())
	fillDet(w.AsFloat32())

	p := convParams{stride: tc.s, group: tc.c}
	if !isDepthwiseFloat32(in, w, p) {
		t.Fatalf("isDepthwiseFloat32 = false for a depthwise conv")
	}
	fast, err := depthwiseConv2DFloat32(in, w, p)
	if err != nil {
		t.Fatalf("depthwise: %v", err)
	}
	slow, err := groupedConv2D(ctx, in, w, tc.s, tc.c)
	if err != nil {
		t.Fatalf("grouped: %v", err)
	}

	fd, sd := fast.AsFloat32(), slow.AsFloat32()
	if len(fd) != len(sd) {
		t.Fatalf("len mismatch %d vs %d", len(fd), len(sd))
	}
	// The direct kernel and groupedConv2D accumulate in the same per-tap order,
	// so they agree to well within float32 rounding (empirically bit-identical
	// for these inputs). A 1e-5 bound keeps margin while still catching any
	// future regression that perturbs the arithmetic.
	for i := range fd {
		if d := fd[i] - sd[i]; d > 1e-5 || d < -1e-5 {
			t.Fatalf("idx %d: fast %v grouped %v (diff %v)", i, fd[i], sd[i], d)
		}
	}
}

func TestDepthwise_MatchesGroupedConv(t *testing.T) {
	ctx := &Context{Backend: cpu.New()}
	for _, tc := range depthwiseCases {
		t.Run(tc.name, func(t *testing.T) { checkDepthwiseVsGrouped(t, ctx, tc) })
	}
}

// TestNonDepthwiseFallsBackToGrouped documents the dispatch boundary: a grouped
// conv that is NOT depthwise (group=2, Cin=4, Cout=4, so weight [4,2,k,k] with
// Cin/group==2 channels per group) must not take the direct depthwise kernel.
// isDepthwiseFloat32 has to report false, and convForward must route to
// groupedConv2D and produce its result unchanged.
func TestNonDepthwiseFallsBackToGrouped(t *testing.T) {
	ctx := &Context{Backend: cpu.New()}
	const n, cin, cout, h, w, k, s, group = 1, 4, 4, 8, 8, 3, 1, 2

	in, err := tensor.NewRaw(tensor.Shape{n, cin, h, w}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	// Grouped (non-depthwise) weight: [Cout, Cin/group, kH, kW] = [4, 2, 3, 3].
	weight, err := tensor.NewRaw(tensor.Shape{cout, cin / group, k, k}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	fillDet(in.AsFloat32())
	fillDet(weight.AsFloat32())

	p := convParams{stride: s, group: group}
	if isDepthwiseFloat32(in, weight, p) {
		t.Fatal("isDepthwiseFloat32 = true for a non-depthwise grouped conv (group=2, Cin=4, Cout=4)")
	}

	// convForward must dispatch to groupedConv2D for this shape, so its output
	// has to match a direct groupedConv2D call bit-for-bit.
	got, err := convForward(ctx, in, weight, p)
	if err != nil {
		t.Fatalf("convForward: %v", err)
	}
	want, err := groupedConv2D(ctx, in, weight, s, group)
	if err != nil {
		t.Fatalf("groupedConv2D: %v", err)
	}
	gd, wd := got.AsFloat32(), want.AsFloat32()
	if len(gd) != len(wd) {
		t.Fatalf("len mismatch %d vs %d", len(gd), len(wd))
	}
	for i := range gd {
		if gd[i] != wd[i] {
			t.Fatalf("idx %d: convForward %v grouped %v", i, gd[i], wd[i])
		}
	}
}

// TestValidateConvShapes_RejectsInputSmallerThanKernel guards the integer-
// truncation edge: with stride 2, (2-3)/2+1 == 1 wrongly passed the old check,
// letting a 2x2 input under a 3x3 kernel reach the conv kernel and read OOB.
func TestValidateConvShapes_RejectsInputSmallerThanKernel(t *testing.T) {
	in, err := tensor.NewRaw(tensor.Shape{1, 4, 2, 2}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	w, err := tensor.NewRaw(tensor.Shape{4, 1, 3, 3}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConvShapes(in, w, convParams{stride: 2, group: 4}); err == nil {
		t.Fatal("expected error for input smaller than kernel, got nil")
	}
}

func benchDepthwise(b *testing.B, n, c, h, w, k, s int) {
	hOut := (h-k)/s + 1
	wOut := (w-k)/s + 1
	in := make([]float32, n*c*h*w)
	weight := make([]float32, c*k*k)
	out := make([]float32, n*c*hOut*wOut)
	fillDet(in)
	fillDet(weight)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		depthwiseConvForwardFloat32(out, in, weight, n, c, h, w, k, k, hOut, wOut, s)
	}
}

// Mirrors the largest BirdNET depthwise layer: 1536 channels, 3x3, stride 1.
func BenchmarkDepthwiseConv_1536ch(b *testing.B) { benchDepthwise(b, 1, 1536, 12, 12, 3, 1) }
func BenchmarkDepthwiseConv_288ch(b *testing.B)  { benchDepthwise(b, 1, 288, 24, 24, 3, 1) }
