//go:build amd64

package operators

//go:generate sh -c "cd _gen/pow && go run . -out ../../pow_simd_amd64.s -stubs ../../pow_simd_stub_gen_amd64.go -pkg operators"

import (
	"math"

	"golang.org/x/sys/cpu"
)

// init wires the vendored AVX2+FMA pow kernel into the dispatch whenever the CPU
// supports AVX2+FMA. It compiles into every default amd64 build (no build tag or
// env flag); dispatch is decided here at startup from runtime CPU detection. CPUs
// without AVX2+FMA leave powConstF32 nil and use the scalar path.
func init() {
	if cpu.X86.HasAVX2 && cpu.X86.HasFMA {
		powConstF32 = powConstAVX2
	}
}

// powConstAVX2 applies the vendored 8-wide pow(x,c) = exp(c*log(x)) kernel to the
// bulk of in and finishes the sub-8 remainder with scalar math.Pow, so any length
// is handled. c is the constant exponent; inputs are expected non-negative.
func powConstAVX2(out, in []float32, c float32) {
	n := len(in)
	n8 := n &^ 7
	if n8 > 0 {
		powConstF32AVX2(out, in, n8, c)
	}
	ex := float64(c)
	for i := n8; i < n; i++ {
		out[i] = float32(math.Pow(float64(in[i]), ex))
	}
}
