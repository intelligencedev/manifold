//go:build amd64

package tensor

import (
	"math"
	"math/rand"
	"testing"

	"golang.org/x/sys/cpu"
)

// refSigmoidF32 is the scalar reference: 1/(1+exp(-x)) computed in float64.
func refSigmoidF32(x float32) float32 {
	return float32(1.0 / (1.0 + math.Exp(float64(-x))))
}

// TestSigmoidF32AVX2MatchesScalar verifies the vendored AVX2 sigmoid kernel
// matches the scalar reference across edge magnitudes and random inputs, well
// inside the model's 1e-3 parity budget.
func TestSigmoidF32AVX2MatchesScalar(t *testing.T) {
	if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
		t.Skip("AVX2+FMA not available on this CPU")
	}

	// Edge magnitudes that exercise saturation, the steep region, and clamping.
	edges := []float32{
		0, 1, -1, 0.5, -0.5, 2, -2, 5, -5, 10, -10, 20, -20,
		87, -87, 88, -88, 89, -89, 100, -100, 1000, -1000,
		1e-4, -1e-4, 0.69314718, -0.69314718, 3.4, -3.4,
	}
	r := rand.New(rand.NewSource(0x5160))
	in := append([]float32(nil), edges...)
	for len(in) < 4096 {
		in = append(in, float32(r.NormFloat64()*8)) // spread across the steep region
	}
	// Pad to a multiple of 8 so the kernel handles the whole slice.
	for len(in)%8 != 0 {
		in = append(in, 0)
	}

	got := make([]float32, len(in))
	for i := range got {
		got[i] = 123.0 // poison: kernel must overwrite
	}
	sigmoidF32AVX2(got, in, len(in))

	var maxAbs, maxRel float64
	for i, x := range in {
		want := refSigmoidF32(x)
		abs := math.Abs(float64(got[i] - want))
		if abs > maxAbs {
			maxAbs = abs
		}
		rel := abs / (1e-9 + math.Abs(float64(want)))
		if rel > maxRel {
			maxRel = rel
		}
		if abs > 1e-4 {
			t.Errorf("x=%g: got %g want %g (abs %.3e)", x, got[i], want, abs)
		}
	}
	t.Logf("max abs diff %.3e, max rel diff %.3e over %d values", maxAbs, maxRel, len(in))
}

// TestSigmoidAVX2Driver checks the Go driver (kernel + scalar tail) over lengths
// that are not multiples of 8.
func TestSigmoidAVX2Driver(t *testing.T) {
	if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
		t.Skip("AVX2+FMA not available on this CPU")
	}
	r := rand.New(rand.NewSource(7))
	for _, n := range []int{0, 1, 3, 7, 8, 9, 15, 16, 17, 31, 100, 1000} {
		in := make([]float32, n)
		for i := range in {
			in[i] = float32(r.NormFloat64() * 4)
		}
		out := make([]float32, n)
		sigmoidAVX2(out, in)
		for i := range in {
			want := refSigmoidF32(in[i])
			if abs := math.Abs(float64(out[i] - want)); abs > 1e-4 {
				t.Fatalf("n=%d i=%d x=%g: got %g want %g (abs %.3e)", n, i, in[i], out[i], want, abs)
			}
		}
	}
}

// TestSigmoidWiredInMatchesCPU verifies the always-on dispatch contract: init
// wires the kernel into sigmoidF32 exactly when the CPU has AVX2+FMA, with no env
// flag or build tag involved. A regression that dropped the CPU check or failed to
// wire it in on a capable CPU would fail here.
func TestSigmoidWiredInMatchesCPU(t *testing.T) {
	want := cpu.X86.HasAVX2 && cpu.X86.HasFMA
	if got := sigmoidF32 != nil; got != want {
		t.Errorf("sigmoidF32 wired in = %v, want %v (AVX2=%v FMA=%v)",
			got, want, cpu.X86.HasAVX2, cpu.X86.HasFMA)
	}
}

// BenchmarkSigmoid compares the scalar float64 path against the AVX2 kernel at a
// representative activation length.
func BenchmarkSigmoid(b *testing.B) {
	const n = 511 * 96 // a representative post-conv activation tile
	r := rand.New(rand.NewSource(3))
	in := make([]float32, n)
	for i := range in {
		in[i] = float32(r.NormFloat64() * 4)
	}
	out := make([]float32, n)
	b.Run("scalar", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for j := range in {
				out[j] = refSigmoidF32(in[j])
			}
		}
	})
	b.Run("simd", func(b *testing.B) {
		if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
			b.Skip("AVX2+FMA not available")
		}
		for i := 0; i < b.N; i++ {
			sigmoidAVX2(out, in)
		}
	})
}
