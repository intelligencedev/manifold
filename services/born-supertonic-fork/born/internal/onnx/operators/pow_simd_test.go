package operators

import (
	"math"
	"math/rand"
	"testing"

	"golang.org/x/sys/cpu"
)

// powTestInput builds a non-negative test slice covering exact zeros plus small,
// unit, and typical magnitudes, which is the domain of the ONNX Pow op on a mel
// spectrogram (base >= 0).
func powTestInput(r *rand.Rand, n int) []float32 {
	s := make([]float32, n)
	for i := range s {
		switch i % 7 {
		case 0:
			s[i] = 0 // pow(0, c>0) == 0
		case 1:
			s[i] = r.Float32() * 0.01 // small
		case 2:
			s[i] = 1
		default:
			s[i] = r.Float32() * 50 // typical magnitude
		}
	}
	return s
}

var powExponents = []float32{0.1905273, 0.22952409, 0.43, 0.5, 1.5, 2.0, 0.1}

// TestPowConstSIMDParity checks the vendored AVX2 pow(x,c) kernel against the
// scalar math.Pow reference across the BirdNET exponents and representative
// non-negative inputs spanning full vector blocks plus sub-8 tails.
func TestPowConstSIMDParity(t *testing.T) {
	if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
		t.Skip("AVX2+FMA not available")
	}
	if powConstF32 == nil {
		t.Skip("vendored SIMD pow not wired")
	}
	r := rand.New(rand.NewSource(0x706f77)) // "pow"
	var globalMax float64
	for _, c := range powExponents {
		for _, n := range []int{1, 7, 8, 9, 17, 256, 49056} {
			in := powTestInput(r, n)
			got := make([]float32, n)
			powConstF32(got, in, c)
			for i := range in {
				want := float32(math.Pow(float64(in[i]), float64(c)))
				d := math.Abs(float64(got[i]-want)) / (1 + math.Abs(float64(want)))
				if d > globalMax {
					globalMax = d
				}
				// exp(c*log(x)) reorders rounding vs float64 math.Pow; 1e-4 relative
				// is comfortably inside the model's 1e-3 parity budget.
				if d > 1e-4 {
					t.Errorf("c=%v n=%d i=%d: rel diff %.3e exceeds 1e-4 (got %v want %v)",
						c, n, i, d, got[i], want)
				}
			}
		}
	}
	t.Logf("max relative error vs math.Pow: %.3e", globalMax)
}

// BenchmarkPowConst compares scalar math.Pow against the vendored SIMD kernel at
// the BirdNET mel-spectrogram Pow size (1*511*96) and a smaller tensor.
func BenchmarkPowConst(b *testing.B) {
	r := rand.New(rand.NewSource(9))
	const c = float32(0.1905273)
	for _, s := range []struct {
		name string
		n    int
	}{
		{"mel_511x96", 49056}, // 1*511*96, the BirdNET Pow tensor
		{"n4096", 4096},
	} {
		in := powTestInput(r, s.n)
		out := make([]float32, s.n)
		b.Run(s.name+"/scalar", func(b *testing.B) {
			ex := float64(c)
			for i := 0; i < b.N; i++ {
				for j := range in {
					out[j] = float32(math.Pow(float64(in[j]), ex))
				}
			}
		})
		b.Run(s.name+"/simd", func(b *testing.B) {
			if powConstF32 == nil {
				b.Skip("no SIMD pow")
			}
			for i := 0; i < b.N; i++ {
				powConstF32(out, in, c)
			}
		})
	}
}

// TestPowConstWiredIn asserts the always-on dispatch contract: the vendored SIMD
// pow is wired exactly when the CPU supports AVX2+FMA, so a dropped init() or a
// flipped CPU check is caught instead of silently skipping the SIMD tests.
func TestPowConstWiredIn(t *testing.T) {
	want := cpu.X86.HasAVX2 && cpu.X86.HasFMA
	if got := powConstF32 != nil; got != want {
		t.Errorf("powConstF32 wired = %v, want %v (HasAVX2=%v HasFMA=%v)",
			got, want, cpu.X86.HasAVX2, cpu.X86.HasFMA)
	}
}
