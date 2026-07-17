// Command activation generates the vendored AVX2+FMA float32 sigmoid kernel for
// the tensor package. Run via `go generate` (see sigmoid_simd_amd64.go); lives in
// a separate module (_gen/activation/go.mod) so avo never enters born's module
// graph. The generated artifacts (sigmoid_simd_amd64.s and its Go stub) are
// committed.
//
// sigmoidF32AVX2 computes out[i] = 1/(1+exp(-in[i])) for n (multiple of 8)
// float32 lanes, 8 at a time. exp is evaluated with the classic Cephes expf
// range reduction (exp(x) = 2^n * P(r), r = x - n*ln2) whose float32 error is
// ~1 ULP, far inside the model's 1e-3 parity budget. The reciprocal uses an
// exact VDIVPS (not VRCPPS) to keep the tail accurate.
package main

import (
	"fmt"
	"math"

	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	"github.com/mmcloughlin/avo/reg"
)

// Cephes single-precision expf constants.
const (
	expHi  = 88.3762626647949
	expLo  = -88.3762626647949
	log2ef = 1.44269504088896341
	expC1  = 0.693359375    // ln2 high part
	expC2  = -2.12194440e-4 // ln2 low part (Cody-Waite); r = x - n*C1 - n*C2
	half   = 0.5
	one    = 1.0
	expP0  = 1.9875691500e-4
	expP1  = 1.3981999507e-3
	expP2  = 8.3334519073e-3
	expP3  = 4.1665795894e-2
	expP4  = 1.6666665459e-1
	expP5  = 5.0000001201e-1
)

var f32pool = map[uint32]Mem{}

// cf returns a RODATA Mem holding val as a single float32, deduplicated by bits.
func cf(val float32) Mem {
	bits := math.Float32bits(val)
	if m, ok := f32pool[bits]; ok {
		return m
	}
	m := GLOBL(fmt.Sprintf("sigf32_%08x", bits), RODATA|NOPTR)
	DATA(0, U32(bits))
	f32pool[bits] = m
	return m
}

var i32pool = map[uint32]Mem{}

// ci returns a RODATA Mem holding val as a single int32, deduplicated.
func ci(val uint32) Mem {
	if m, ok := i32pool[val]; ok {
		return m
	}
	m := GLOBL(fmt.Sprintf("sigi32_%08x", val), RODATA|NOPTR)
	DATA(0, U32(val))
	i32pool[val] = m
	return m
}

// bcf broadcasts a float32 constant into a fresh YMM.
func bcf(val float32) reg.VecVirtual {
	y := YMM()
	VBROADCASTSS(cf(val), y)
	return y
}

func main() {
	TEXT("sigmoidF32AVX2", NOSPLIT, "func(out, in []float32, n int)")
	Doc("sigmoidF32AVX2 computes out[i] = 1/(1+exp(-in[i])) for the first n (multiple of 8)",
		"float32 lanes using an AVX2+FMA Cephes expf approximation. The caller handles any",
		"sub-8 remainder in Go.")
	Pragma("noescape")

	outPtr := Load(Param("out").Base(), GP64())
	inPtr := Load(Param("in").Base(), GP64())
	n := Load(Param("n"), GP64())

	blocks := GP64()
	MOVQ(n, blocks)
	SHRQ(Imm(3), blocks) // blocks = n / 8

	// Sign mask (-0.0 = 0x80000000) to negate x: arg = -x for sigmoid.
	signMask := bcf(math.Float32frombits(0x80000000))

	Label("loop")
	CMPQ(blocks, Imm(0))
	JE(LabelRef("done"))

	x := YMM()
	VMOVUPS(Mem{Base: inPtr}, x)
	VXORPS(signMask, x, x) // x = -x (the exp argument)

	// Clamp to the expf valid range.
	VMINPS(bcf(expHi), x, x)
	VMAXPS(bcf(expLo), x, x)

	// fx = floor(x*log2ef + 0.5)
	fx := YMM()
	VMULPS(bcf(log2ef), x, fx)
	VADDPS(bcf(half), fx, fx)
	VROUNDPS(Imm(1), fx, fx) // round toward -inf (floor)

	// r = x - fx*C1 - fx*C2  (Cody-Waite reduction)
	VFNMADD231PS(bcf(expC1), fx, x) // x = x - fx*C1
	VFNMADD231PS(bcf(expC2), fx, x) // x = x - fx*C2  (C2 < 0)
	r := x

	// z = r*r
	z := YMM()
	VMULPS(r, r, z)

	// Horner: y = ((((p0*r+p1)*r+p2)*r+p3)*r+p4)*r+p5
	y := bcf(expP0)
	for _, p := range []float32{expP1, expP2, expP3, expP4, expP5} {
		VMULPS(r, y, y)
		VADDPS(bcf(p), y, y)
	}
	// y = y*z + r + 1
	VMULPS(z, y, y)
	VADDPS(r, y, y)
	VADDPS(bcf(one), y, y)

	// 2^n via integer exponent: pow2 = float bits ((int(fx)+127) << 23)
	ni := YMM()
	VCVTTPS2DQ(fx, ni) // fx is integer-valued, truncate is exact
	pow127 := YMM()
	VPBROADCASTD(ci(127), pow127)
	VPADDD(pow127, ni, ni)
	VPSLLD(Imm(23), ni, ni)

	// exp(x) = y * 2^n
	VMULPS(ni, y, y)

	// sigmoid = 1/(1+exp(x)); exact reciprocal.
	oneV := bcf(one)
	denom := YMM()
	VADDPS(oneV, y, denom) // 1 + exp
	res := YMM()
	VDIVPS(denom, oneV, res) // res = 1 / denom
	VMOVUPS(res, Mem{Base: outPtr})

	ADDQ(Imm(32), inPtr)
	ADDQ(Imm(32), outPtr)
	DECQ(blocks)
	JMP(LabelRef("loop"))

	Label("done")
	VZEROUPPER()
	RET()

	Generate()
}
