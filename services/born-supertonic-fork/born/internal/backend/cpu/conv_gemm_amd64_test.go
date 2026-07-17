//go:build amd64 && !goexperiment.simd

package cpu

import (
	"math"
	"math/rand"
	"testing"

	"golang.org/x/sys/cpu"
)

// naiveColBufMatMul is an independent reference for matMulColBufFloat32:
// out[i*colHeight+j] = sum_k kernel[i*colWidth+k] * colBuf[j*colWidth+k].
func naiveColBufMatMul(kernel, colBuf []float32, cOut, colHeight, colWidth int) []float32 {
	out := make([]float32, cOut*colHeight)
	for i := 0; i < cOut; i++ {
		for j := 0; j < colHeight; j++ {
			var s float32
			for k := 0; k < colWidth; k++ {
				s += kernel[i*colWidth+k] * colBuf[j*colWidth+k]
			}
			out[i*colHeight+j] = s
		}
	}
	return out
}

// TestMatMulColBufGemmDispatch verifies the conv im2col GEMM routes profitable
// shapes through the SIMD gemm kernel (via a colBuf transpose) and keeps tiny
// depthwise-style shapes on scalar, with correct results either way. A sentinel
// proves the SIMD path is actually taken so the test is not vacuous.
func TestMatMulColBufGemmDispatch(t *testing.T) {
	if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
		t.Skip("AVX2+FMA not available on this CPU")
	}
	r := rand.New(rand.NewSource(0x636f6e76)) // "conv"

	var called bool
	withGemmF32(t, func(c, a, b []float32, m, k, n int) {
		called = true
		gemmAVX2F32(c, a, b, m, k, n)
	})

	cases := []struct {
		name                      string
		cOut, colHeight, colWidth int
		wantGemm                  bool
	}{
		{"regular conv routes to gemm", 64, 384, 288, true}, // 7.1M >= blockThreshold, colH >= 16
		{"depthwise cOut=1 stays scalar", 1, 24, 9, false},  // 216 < blockThreshold
		{"narrow colHeight<16 stays scalar", 512, 8, 64, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			kernel := randSliceF32(r, tc.cOut*tc.colWidth)
			colBuf := randSliceF32(r, tc.colHeight*tc.colWidth)
			out := make([]float32, tc.cOut*tc.colHeight)
			for i := range out {
				out[i] = 999.0 // poison
			}
			matMulColBufFloat32(out, kernel, colBuf, tc.cOut, tc.colHeight, tc.colWidth)
			want := naiveColBufMatMul(kernel, colBuf, tc.cOut, tc.colHeight, tc.colWidth)

			if called != tc.wantGemm {
				t.Errorf("gemm path taken = %v, want %v", called, tc.wantGemm)
			}
			var maxDiff float64
			for i := range want {
				d := math.Abs(float64(out[i]-want[i])) / (1 + math.Abs(float64(want[i])))
				if d > maxDiff {
					maxDiff = d
				}
			}
			if maxDiff > 1e-4 {
				t.Errorf("max rel diff %.3e exceeds 1e-4", maxDiff)
			}
		})
	}
}

// BenchmarkMatMulColBuf compares the scalar conv im2col GEMM against the SIMD
// path (colBuf transpose + reused GEMM kernel) at representative regular-conv
// shapes from the BirdNET v2.4 model.
func BenchmarkMatMulColBuf(b *testing.B) {
	shapes := []struct {
		name             string
		cOut, colH, colW int
	}{
		{"conv_864x96x108", 864, 96, 108},
		{"conv_288x384x72", 288, 384, 72},
		{"conv_1536x24x192", 1536, 24, 192},
	}
	r := rand.New(rand.NewSource(11))
	for _, s := range shapes {
		kernel := randSliceF32(r, s.cOut*s.colW)
		colBuf := randSliceF32(r, s.colH*s.colW)
		out := make([]float32, s.cOut*s.colH)
		b.Run(s.name+"/scalar", func(b *testing.B) {
			prev := gemmF32
			gemmF32 = nil
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				matMulColBufFloat32(out, kernel, colBuf, s.cOut, s.colH, s.colW)
			}
			b.StopTimer()
			gemmF32 = prev
		})
		b.Run(s.name+"/simd", func(b *testing.B) {
			if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
				b.Skip("AVX2+FMA not available")
			}
			prev := gemmF32
			gemmF32 = gemmAVX2F32
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				matMulColBufFloat32(out, kernel, colBuf, s.cOut, s.colH, s.colW)
			}
			b.StopTimer()
			gemmF32 = prev
		})
	}
}
