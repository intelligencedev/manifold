package cpu

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// convScratchCase describes a regular (non-1x1) convolution that exercises the
// im2col path in conv2dFloat32 / conv2dFloat64 (colBuf + matmul + rearrange).
type convScratchCase struct {
	name                       string
	n, cIn, h, w, cOut, kh, kw int
	stride, padding            int
}

// Cases cover both specialized im2col paths (stride=1/pad=0 and the general
// path), at a size that routes through the SIMD GEMM and one that stays scalar.
var convScratchCases = []convScratchCase{
	{"stride1nopad_gemm", 1, 8, 16, 16, 32, 3, 3, 1, 0},  // colHeight*cOut*colWidth >= blockThreshold -> SIMD GEMM
	{"stride1nopad_scalar", 1, 4, 10, 10, 6, 3, 3, 1, 0}, // small -> scalar matmul
	{"padded_general", 1, 8, 12, 12, 16, 3, 3, 1, 1},     // general path with padding
	{"strided_general", 1, 8, 16, 16, 16, 3, 3, 2, 0},    // general path with stride 2
}

// buildConvScratch builds a pre-allocated output plus input/kernel tensors and
// the ConvDims for a case, so a test can call conv2dFloat32/conv2dFloat64
// directly (white-box) without the per-call output-tensor allocation that
// backend.Conv2D adds.
func buildConvScratch(c convScratchCase, dt tensor.DataType) (output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	hOut := (c.h+2*c.padding-c.kh)/c.stride + 1
	wOut := (c.w+2*c.padding-c.kw)/c.stride + 1
	input, _ = tensor.NewRaw(tensor.Shape{c.n, c.cIn, c.h, c.w}, dt, tensor.CPU)
	kernel, _ = tensor.NewRaw(tensor.Shape{c.cOut, c.cIn, c.kh, c.kw}, dt, tensor.CPU)
	output, _ = tensor.NewRaw(tensor.Shape{c.n, c.cOut, hOut, wOut}, dt, tensor.CPU)
	fillPointwiseConv(input, func(i int) float64 { return float64((i%13)-6) * 0.25 })
	fillPointwiseConv(kernel, func(i int) float64 { return float64((i%7)-3) * 0.5 })
	dims = &ConvDims{
		N: c.n, CIn: c.cIn, H: c.h, W: c.w,
		COut: c.cOut, KH: c.kh, KW: c.kw,
		HOut: hOut, WOut: wOut,
		Stride: c.stride, Padding: c.padding,
	}
	return output, input, kernel, dims
}

func runConvScratch(dt tensor.DataType, output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	switch dt {
	case tensor.Float32:
		conv2dFloat32(output, input, kernel, dims)
	case tensor.Float64:
		conv2dFloat64(output, input, kernel, dims)
	}
}

// TestConv2DScratchAllocFree verifies the im2col conv path recycles its colBuf
// and matmul-output scratch from a pool: after the pool warms, a convolution
// into a pre-allocated output allocates nothing per call. Guarded with
// testing.Short because testing.AllocsPerRun over a shared sync.Pool is flaky
// under -short -race (CI runs -short -race).
func TestConv2DScratchAllocFree(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("AllocsPerRun over a shared sync.Pool is unreliable under -short and the race detector")
	}
	for _, dt := range []tensor.DataType{tensor.Float32, tensor.Float64} {
		for _, c := range convScratchCases {
			t.Run(dt.String()+"/"+c.name, func(t *testing.T) {
				output, input, kernel, dims := buildConvScratch(c, dt)
				allocs := testing.AllocsPerRun(20, func() {
					runConvScratch(dt, output, input, kernel, dims)
				})
				if allocs != 0 {
					t.Errorf("conv allocates %.0f scratch buffers/op, want 0", allocs)
				}
			})
		}
	}
}

// convDataEqual reports the first index where two same-dtype conv outputs differ
// bit-for-bit, or (-1, true) if identical.
func convDataEqual(a, b *tensor.RawTensor) (idx int, equal bool) {
	switch a.DType() {
	case tensor.Float32:
		ad, bd := a.AsFloat32(), b.AsFloat32()
		for i := range ad {
			if ad[i] != bd[i] {
				return i, false
			}
		}
	case tensor.Float64:
		ad, bd := a.AsFloat64(), b.AsFloat64()
		for i := range ad {
			if ad[i] != bd[i] {
				return i, false
			}
		}
	}
	return -1, true
}

// TestConv2DPooledReuseDeterministic runs each convolution repeatedly, sharing
// the recycled scratch pools across calls, and asserts every run is bit-for-bit
// identical to the first. A dirty buffer leaking through reuse would diverge.
func TestConv2DPooledReuseDeterministic(t *testing.T) {
	backend := New()
	for _, dt := range []tensor.DataType{tensor.Float32, tensor.Float64} {
		for _, c := range convScratchCases {
			t.Run(dt.String()+"/"+c.name, func(t *testing.T) {
				_, input, kernel, _ := buildConvScratch(c, dt)
				first := backend.Conv2D(input, kernel, c.stride, c.padding)
				for i := 0; i < 8; i++ {
					got := backend.Conv2D(input, kernel, c.stride, c.padding)
					if idx, ok := convDataEqual(first, got); !ok {
						t.Fatalf("run %d diverged from first at index %d", i+1, idx)
					}
				}
			})
		}
	}
}

// poisonConvPools pre-dirties the recycled scratch pools for dtype dt with a
// sentinel at exactly the sizes the next conv of this shape will request, so the
// conv reuses the poisoned buffers. If im2col and the matmul fully overwrite
// their buffers (the pooling safety contract), the sentinel never reaches the
// output.
func poisonConvPools(dt tensor.DataType, colN, outN int) {
	const sentinel = -123456.0
	switch dt {
	case tensor.Float32:
		cp := poolScratch[float32](&convColPoolF32, colN)
		for i := range *cp {
			(*cp)[i] = sentinel
		}
		convColPoolF32.Put(cp)
		mp := poolScratch[float32](&convOutPoolF32, outN)
		for i := range *mp {
			(*mp)[i] = sentinel
		}
		convOutPoolF32.Put(mp)
	case tensor.Float64:
		cp := poolScratch[float64](&convColPoolF64, colN)
		for i := range *cp {
			(*cp)[i] = sentinel
		}
		convColPoolF64.Put(cp)
		mp := poolScratch[float64](&convOutPoolF64, outN)
		for i := range *mp {
			(*mp)[i] = sentinel
		}
		convOutPoolF64.Put(mp)
	}
}

// TestConv2DPooledPoisonedOverwrite poisons the recycled scratch pools with a
// sentinel, then asserts the conv output is bit-identical to a clean run. This
// proves im2col and the matmul fully overwrite the recycled buffers, so dirty
// reuse is safe (the pattern proven for the GEMM kernel in #96 and #99).
func TestConv2DPooledPoisonedOverwrite(t *testing.T) {
	backend := New()
	for _, dt := range []tensor.DataType{tensor.Float32, tensor.Float64} {
		for _, c := range convScratchCases {
			t.Run(dt.String()+"/"+c.name, func(t *testing.T) {
				_, input, kernel, dims := buildConvScratch(c, dt)
				clean := backend.Conv2D(input, kernel, c.stride, c.padding)

				colN := dims.CIn * dims.KH * dims.KW * (dims.N * dims.HOut * dims.WOut)
				outN := dims.COut * (dims.N * dims.HOut * dims.WOut)
				poisonConvPools(dt, colN, outN)

				got := backend.Conv2D(input, kernel, c.stride, c.padding)
				if idx, ok := convDataEqual(clean, got); !ok {
					t.Fatalf("poisoned scratch leaked into output at index %d", idx)
				}
			})
		}
	}
}

// TestConv2DPooledMatchesMock checks the pooled im2col path against the naive
// MockBackend oracle across the regular-conv shapes (both specialized paths,
// scalar and SIMD-GEMM routing), for both dtypes.
func TestConv2DPooledMatchesMock(t *testing.T) {
	backend := New()
	mock := tensor.NewMockBackend()
	for _, dt := range []tensor.DataType{tensor.Float32, tensor.Float64} {
		// Relative tolerance: float32 GEMM reorders FMA accumulation; float64 is
		// effectively exact.
		tol := 1e-4
		if dt == tensor.Float64 {
			tol = 1e-12
		}
		for _, c := range convScratchCases {
			t.Run(dt.String()+"/"+c.name, func(t *testing.T) {
				_, input, kernel, _ := buildConvScratch(c, dt)
				got := backend.Conv2D(input, kernel, c.stride, c.padding)
				want := mock.Conv2D(input, kernel, c.stride, c.padding)
				if !got.Shape().Equal(want.Shape()) {
					t.Fatalf("shape: CPU=%v Mock=%v", got.Shape(), want.Shape())
				}
				if d, idx := maxPointwiseConvDiff(got, want); d > tol*(1+absAt(want, idx)) {
					t.Errorf("idx %d: abs diff %.3g exceeds rel tol %.3g", idx, d, tol)
				}
			})
		}
	}
}

// absAt returns |value| at flat index i of a same-dtype tensor (helper for a
// relative-tolerance check).
func absAt(t *tensor.RawTensor, i int) float64 {
	if i < 0 {
		return 0
	}
	switch t.DType() {
	case tensor.Float32:
		return math.Abs(float64(t.AsFloat32()[i]))
	case tensor.Float64:
		return math.Abs(t.AsFloat64()[i])
	}
	return 0
}

func benchConv2DIm2col(b *testing.B, n, cIn, h, w, cOut, k, stride, pad int) {
	backend := New()
	input := tensor.Randn[float32](tensor.Shape{n, cIn, h, w}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{cOut, cIn, k, k}, backend).Raw()
	b.ResetTimer()
	for b.Loop() {
		backend.Conv2D(input, kernel, stride, pad)
	}
}

// Regular (KxK, K>1) conv layers that exercise the im2col + matmul + rearrange
// path whose colBuf and matmul-output scratch are pooled. Run with -benchmem to
// see the per-conv allocation and B/op drop from recycling those buffers.
func BenchmarkConv2D_Im2col_GEMM(b *testing.B)    { benchConv2DIm2col(b, 1, 32, 64, 64, 64, 3, 1, 1) }
func BenchmarkConv2D_Im2col_Deep(b *testing.B)    { benchConv2DIm2col(b, 1, 64, 32, 32, 128, 3, 1, 1) }
func BenchmarkConv2D_Im2col_Strided(b *testing.B) { benchConv2DIm2col(b, 1, 16, 64, 64, 32, 3, 2, 1) }
