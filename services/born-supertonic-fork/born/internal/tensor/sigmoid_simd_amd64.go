//go:build amd64

package tensor

//go:generate sh -c "cd _gen/activation && go run . -out ../../sigmoid_simd_amd64.s -stubs ../../sigmoid_simd_stub_gen_amd64.go -pkg tensor"

import (
	"math"

	"golang.org/x/sys/cpu"
)

// init wires the vendored AVX2+FMA sigmoid kernel into the dispatch whenever the
// CPU supports AVX2+FMA. It compiles into every default amd64 build (no build tag
// or env flag needed), and dispatch is decided here at startup from runtime CPU
// detection; CPUs without AVX2+FMA leave sigmoidF32 nil and use the scalar path.
func init() {
	if cpu.X86.HasAVX2 && cpu.X86.HasFMA {
		sigmoidF32 = sigmoidAVX2
	}
}

// sigmoidAVX2 applies the vendored 8-wide kernel to the bulk of in and finishes
// the sub-8 remainder with the scalar reference, so any length is handled.
func sigmoidAVX2(out, in []float32) {
	n := len(in)
	n8 := n &^ 7
	if n8 > 0 {
		sigmoidF32AVX2(out, in, n8)
	}
	for i := n8; i < n; i++ {
		out[i] = float32(1.0 / (1.0 + math.Exp(float64(-in[i]))))
	}
}
