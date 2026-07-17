package operators

import (
	"math"
	"math/rand"
	"testing"

	"golang.org/x/sys/cpu"
)

// depthwise3x3NaiveRef is an independent reference for the stride=1, padding=0
// 3x3 depthwise convolution: each (n,c) plane is convolved with channel c's 9
// taps. Used as the parity oracle for the SIMD kernel.
func depthwise3x3NaiveRef(out, in, weight []float32, n, c, hp, wp, hOut, wOut int) {
	planeIn := hp * wp
	planeOut := hOut * wOut
	for plane := 0; plane < n*c; plane++ {
		inBase := plane * planeIn
		outBase := plane * planeOut
		w := weight[(plane%c)*9 : (plane%c)*9+9]
		for oh := 0; oh < hOut; oh++ {
			for ow := 0; ow < wOut; ow++ {
				var sum float32
				for kh := 0; kh < 3; kh++ {
					for kw := 0; kw < 3; kw++ {
						sum += in[inBase+(oh+kh)*wp+(ow+kw)] * w[kh*3+kw]
					}
				}
				out[outBase+oh*wOut+ow] = sum
			}
		}
	}
}

var depthwiseParityShapes = []struct {
	name         string
	n, c, hp, wp int
}{
	{"1ch_tiny", 1, 1, 5, 5},     // wOut=3, sub-8 only (all scalar tail)
	{"3ch_wOut8", 1, 3, 8, 10},   // wOut=8, exactly one vector block
	{"4ch_wOut15", 1, 4, 16, 17}, // wOut=15: one block + overlapping tail (off=7)
	{"batch_5ch", 2, 5, 12, 11},  // wOut=9: one block + overlapping tail (off=1), batched
	{"8ch_square", 1, 8, 18, 18}, // wOut=16, two blocks
	{"wide", 1, 2, 6, 34},        // wOut=32, four blocks
	{"1ch_wOut1", 1, 1, 4, 3},    // wOut=1, degenerate tail
}

// TestDepthwise3x3SIMDParity verifies the vendored AVX2 depthwise kernel matches
// the naive reference across channel counts, batch, and output widths that span
// full vector blocks plus sub-8 tails.
func TestDepthwise3x3SIMDParity(t *testing.T) {
	if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
		t.Skip("AVX2+FMA not available on this CPU")
	}
	if depthwise3x3F32 == nil {
		t.Skip("vendored SIMD depthwise not wired on this build")
	}
	r := rand.New(rand.NewSource(0x6477)) // "dw"
	for _, s := range depthwiseParityShapes {
		t.Run(s.name, func(t *testing.T) {
			hOut, wOut := s.hp-2, s.wp-2
			in := randSliceDW(r, s.n*s.c*s.hp*s.wp)
			weight := randSliceDW(r, s.c*9)
			got := make([]float32, s.n*s.c*hOut*wOut)
			want := make([]float32, s.n*s.c*hOut*wOut)

			depthwise3x3F32(got, in, weight, s.n, s.c, s.hp, s.wp, hOut, wOut)
			depthwise3x3NaiveRef(want, in, weight, s.n, s.c, s.hp, s.wp, hOut, wOut)

			var maxRel float64
			for i := range want {
				d := math.Abs(float64(got[i]-want[i])) / (1 + math.Abs(float64(want[i])))
				if d > maxRel {
					maxRel = d
				}
			}
			if maxRel > 1e-5 {
				t.Errorf("max rel diff %.3e exceeds 1e-5", maxRel)
			}
		})
	}
}

// TestDepthwise3x3WiredIn asserts the always-on dispatch contract: the vendored
// SIMD depthwise is wired exactly when the CPU supports AVX2+FMA, and never
// otherwise. This catches a dropped init() or a flipped CPU check, which would
// silently turn the SIMD path off and make every other depthwise test t.Skip
// while CI stays green.
func TestDepthwise3x3WiredIn(t *testing.T) {
	want := cpu.X86.HasAVX2 && cpu.X86.HasFMA
	if got := depthwise3x3F32 != nil; got != want {
		t.Errorf("depthwise3x3F32 wired = %v, want %v (HasAVX2=%v HasFMA=%v)",
			got, want, cpu.X86.HasAVX2, cpu.X86.HasFMA)
	}
}

// TestDepthwise3x3DispatchStride verifies depthwiseConvForward3x3Float32 routes
// stride=1 through the SIMD kernel and keeps stride>1 on the scalar path, with
// correct results either way. A sentinel proves the SIMD path is actually taken.
func TestDepthwise3x3DispatchStride(t *testing.T) {
	if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA || depthwise3x3F32 == nil {
		t.Skip("vendored SIMD depthwise not available")
	}
	r := rand.New(rand.NewSource(0x73))
	n, c, hp, wp := 1, 4, 16, 16

	for _, s := range []int{1, 2} {
		hOut := (hp-3)/s + 1
		wOut := (wp-3)/s + 1
		in := randSliceDW(r, n*c*hp*wp)
		weight := randSliceDW(r, c*9)
		got := make([]float32, n*c*hOut*wOut)

		var called bool
		prev := depthwise3x3F32
		depthwise3x3F32 = func(o, i, w []float32, nn, cc, h, ww, ho, wo int) {
			called = true
			prev(o, i, w, nn, cc, h, ww, ho, wo)
		}
		depthwiseConvForward3x3Float32(got, in, weight, n, c, hp, wp, hOut, wOut, s)
		depthwise3x3F32 = prev

		if want := s == 1; called != want {
			t.Errorf("stride=%d: SIMD path taken=%v, want %v", s, called, want)
		}
		want := make([]float32, n*c*hOut*wOut)
		depthwise3x3StrideRef(want, in, weight, n, c, hp, wp, hOut, wOut, s)
		for i := range want {
			if d := math.Abs(float64(got[i] - want[i])); d > 1e-4 {
				t.Errorf("stride=%d idx %d: got %.5f want %.5f", s, i, got[i], want[i])
				break
			}
		}
	}
}

// depthwise3x3StrideRef is a strided naive reference (stride s, padding 0).
func depthwise3x3StrideRef(out, in, weight []float32, n, c, hp, wp, hOut, wOut, s int) {
	planeIn := hp * wp
	planeOut := hOut * wOut
	for plane := 0; plane < n*c; plane++ {
		inBase := plane * planeIn
		outBase := plane * planeOut
		w := weight[(plane%c)*9 : (plane%c)*9+9]
		for oh := 0; oh < hOut; oh++ {
			for ow := 0; ow < wOut; ow++ {
				var sum float32
				for kh := 0; kh < 3; kh++ {
					for kw := 0; kw < 3; kw++ {
						sum += in[inBase+(oh*s+kh)*wp+(ow*s+kw)] * w[kh*3+kw]
					}
				}
				out[outBase+oh*wOut+ow] = sum
			}
		}
	}
}

func randSliceDW(r *rand.Rand, n int) []float32 {
	s := make([]float32, n)
	for i := range s {
		s[i] = r.Float32()*2 - 1
	}
	return s
}

// BenchmarkDepthwise3x3 compares the scalar 3x3 depthwise against the SIMD path
// at representative EfficientNet/BirdNET depthwise shapes (stride 1).
func BenchmarkDepthwise3x3(b *testing.B) {
	shapes := []struct {
		name         string
		n, c, hp, wp int
	}{
		{"96ch_64x64", 1, 96, 66, 66},   // wOut=64, no tail
		{"144ch_32x32", 1, 144, 34, 34}, // wOut=32, no tail
		{"480ch_16x16", 1, 480, 18, 18}, // wOut=16, no tail
		{"112ch_28x28", 1, 112, 30, 30}, // wOut=28: 3 blocks + overlapping tail (off=20)
		{"40ch_14x14", 1, 40, 16, 16},   // wOut=14: 1 block + overlapping tail (off=6)
	}
	r := rand.New(rand.NewSource(7))
	for _, s := range shapes {
		hOut, wOut := s.hp-2, s.wp-2
		in := randSliceDW(r, s.n*s.c*s.hp*s.wp)
		weight := randSliceDW(r, s.c*9)
		out := make([]float32, s.n*s.c*hOut*wOut)
		b.Run(s.name+"/scalar", func(b *testing.B) {
			prev := depthwise3x3F32
			depthwise3x3F32 = nil
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				depthwiseConvForward3x3Float32(out, in, weight, s.n, s.c, s.hp, s.wp, hOut, wOut, 1)
			}
			b.StopTimer()
			depthwise3x3F32 = prev
		})
		b.Run(s.name+"/simd", func(b *testing.B) {
			if depthwise3x3F32 == nil {
				b.Skip("no SIMD depthwise")
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				depthwiseConvForward3x3Float32(out, in, weight, s.n, s.c, s.hp, s.wp, hOut, wOut, 1)
			}
		})
	}
}
