// Command pow generates the vendored AVX2+FMA float32 pow(x, c) kernel for the
// operators package. Run via `go generate` (see pow_simd_amd64.go); lives in a
// separate module (_gen/pow/go.mod) so avo never enters born's module graph. The
// generated artifacts (pow_simd_amd64.s and its Go stub) are committed.
//
// powConstF32AVX2 computes out[i] = pow(in[i], c) = exp(c*log(in[i])) for n
// (multiple of 8) float32 lanes, 8 at a time, with a constant exponent c. log is
// the Cephes single-precision logf (frexp + minimax polynomial) and exp is the
// Cephes expf with Cody-Waite range reduction; both are ~1 ULP in float32, so the
// composed result is well inside the model's 1e-3 parity budget. Non-positive
// inputs are flushed to 0 (pow(0, c>0) == 0; the bitwise frexp cannot represent
// 0/negatives), matching math.Pow over the non-negative ONNX Pow domain.
package main

import (
	"fmt"
	"math"

	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	"github.com/mmcloughlin/avo/reg"
)

// Cephes single-precision logf constants (Moshier).
const (
	logP0  = 7.0376836292e-2
	logP1  = -1.1514610310e-1
	logP2  = 1.1676998740e-1
	logP3  = -1.2420140846e-1
	logP4  = 1.4249322787e-1
	logP5  = -1.6668057665e-1
	logP6  = 2.0000714765e-1
	logP7  = -2.4999993993e-1
	logP8  = 3.3333331174e-1
	sqrtHF = 0.7071067690849304

	// ln(2) split, shared by logf and the expf range reduction.
	ln2hi = 0.693359375
	ln2lo = -2.12194440e-4

	// Cephes expf constants.
	expHi  = 88.3762626647949
	expLo  = -88.3762626647949
	log2ef = 1.44269504088896341
	expP0  = 1.9875691500e-4
	expP1  = 1.3981999507e-3
	expP2  = 8.3334519073e-3
	expP3  = 4.1665795894e-2
	expP4  = 1.6666665459e-1
	expP5  = 5.0000001201e-1

	half = 0.5
	one  = 1.0
)

var f32pool = map[uint32]Mem{}

// cf returns a RODATA Mem holding val as a single float32, deduplicated by bits.
func cf(val float32) Mem {
	bits := math.Float32bits(val)
	if m, ok := f32pool[bits]; ok {
		return m
	}
	m := GLOBL(fmt.Sprintf("powf32_%08x", bits), RODATA|NOPTR)
	DATA(0, U32(bits))
	f32pool[bits] = m
	return m
}

var i32pool = map[uint32]Mem{}

// ci returns a RODATA Mem holding val as a single uint32, deduplicated.
func ci(val uint32) Mem {
	if m, ok := i32pool[val]; ok {
		return m
	}
	m := GLOBL(fmt.Sprintf("powi32_%08x", val), RODATA|NOPTR)
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

// bci broadcasts a uint32 constant into a fresh YMM for integer-lane ops (AVX2
// VP* instructions have no embedded broadcast, so materialize a full vector).
func bci(val uint32) reg.VecVirtual {
	y := YMM()
	VPBROADCASTD(ci(val), y)
	return y
}

func main() {
	TEXT("powConstF32AVX2", NOSPLIT, "func(out, in []float32, n int, c float32)")
	Doc("powConstF32AVX2 computes out[i] = pow(in[i], c) = exp(c*log(in[i])) for the",
		"first n (multiple of 8) float32 lanes using AVX2+FMA Cephes logf and expf.",
		"Non-positive inputs are flushed to 0. The caller handles any sub-8 remainder.")
	Pragma("noescape")

	outPtr := Load(Param("out").Base(), GP64())
	inPtr := Load(Param("in").Base(), GP64())
	n := Load(Param("n"), GP64())
	cScalar := Load(Param("c"), XMM())
	cVec := YMM()
	VBROADCASTSS(cScalar, cVec)

	blocks := GP64()
	MOVQ(n, blocks)
	SHRQ(Imm(3), blocks) // n / 8

	zero := YMM()
	VXORPS(zero, zero, zero)

	Label("loop")
	CMPQ(blocks, Imm(0))
	JE(LabelRef("done"))

	x := YMM()
	VMOVUPS(Mem{Base: inPtr}, x)

	// Mask of strictly-positive lanes; applied at the end to flush x<=0 to 0.
	posMask := YMM()
	VCMPPS(Imm(0x1e), zero, x, posMask) // GT_OQ: x > 0

	// ---- Cephes logf(x) -> lg ----
	// frexp: e = (bits >> 23) - 126; m = (bits & 0x007fffff) | 0x3f000000 (in [0.5,1)).
	ei := YMM()
	VPSRLD(Imm(23), x, ei)
	VPSUBD(bci(126), ei, ei)
	ef := YMM()
	VCVTDQ2PS(ei, ef)

	m := YMM()
	VPAND(bci(0x007fffff), x, m)
	VPOR(bci(0x3f000000), m, m)

	// Branchless SQRTHF adjust: if m < SQRTHF { e -= 1; m = 2m-1 } else { m = m-1 }.
	ltMask := YMM()
	VCMPPS(Imm(1), bcf(sqrtHF), m, ltMask) // LT_OS: m < SQRTHF
	adj := YMM()
	VANDPS(ltMask, bcf(one), adj)
	VSUBPS(adj, ef, ef)    // e -= (m<SQRTHF)
	VANDPS(ltMask, m, adj) // adj = (m<SQRTHF) ? m : 0
	VADDPS(adj, m, m)      // m += adj  -> 2m (true) or m (false)
	VSUBPS(bcf(one), m, m) // m -= 1

	z := YMM()
	VMULPS(m, m, z) // z = m^2

	// Horner: poly = (((((((P0*m+P1)*m+P2)...)*m+P8)
	poly := bcf(logP0)
	for _, p := range []float32{logP1, logP2, logP3, logP4, logP5, logP6, logP7, logP8} {
		VFMADD213PS(bcf(p), m, poly) // poly = poly*m + p
	}
	lg := YMM()
	VMULPS(poly, m, lg) // poly * m
	VMULPS(lg, z, lg)   // * m^2  -> poly * m^3

	// Corrections: lg += e*ln2lo - 0.5*z + m + e*ln2hi.
	VFMADD231PS(bcf(ln2lo), ef, lg) // lg += e*ln2lo
	VFMADD231PS(bcf(-half), z, lg)  // lg += -0.5*z
	VADDPS(m, lg, lg)               // lg += m
	VFMADD231PS(bcf(ln2hi), ef, lg) // lg += e*ln2hi

	// ---- arg = c * log(x) ----
	arg := YMM()
	VMULPS(cVec, lg, arg)

	// ---- Cephes expf(arg) -> y ----
	VMINPS(bcf(expHi), arg, arg)
	VMAXPS(bcf(expLo), arg, arg)

	fx := YMM()
	VMULPS(bcf(log2ef), arg, fx)
	VADDPS(bcf(half), fx, fx)
	VROUNDPS(Imm(1), fx, fx) // floor

	VFNMADD231PS(bcf(ln2hi), fx, arg) // arg -= fx*ln2hi
	VFNMADD231PS(bcf(ln2lo), fx, arg) // arg -= fx*ln2lo
	r := arg

	z2 := YMM()
	VMULPS(r, r, z2)

	y := bcf(expP0)
	for _, p := range []float32{expP1, expP2, expP3, expP4, expP5} {
		VMULPS(r, y, y)
		VADDPS(bcf(p), y, y)
	}
	VMULPS(z2, y, y)
	VADDPS(r, y, y)
	VADDPS(bcf(one), y, y)

	// 2^fx via integer exponent: ((int(fx)+127) << 23) reinterpreted as float.
	ni := YMM()
	VCVTTPS2DQ(fx, ni)
	VPADDD(bci(127), ni, ni)
	VPSLLD(Imm(23), ni, ni)
	VMULPS(ni, y, y) // exp = y * 2^fx

	// Flush non-positive inputs to 0.
	VANDPS(posMask, y, y)
	VMOVUPS(y, Mem{Base: outPtr})

	ADDQ(Imm(32), inPtr)
	ADDQ(Imm(32), outPtr)
	DECQ(blocks)
	JMP(LabelRef("loop"))

	Label("done")
	VZEROUPPER()
	RET()

	Generate()
}
