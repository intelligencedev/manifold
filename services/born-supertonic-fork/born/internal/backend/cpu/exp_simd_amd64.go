//go:build amd64 && goexperiment.simd

package cpu

import (
	"math"
	"simd/archsimd"
)

// Declared here for amd64+goexperiment.simd builds; the stub file provides the
// same declaration for all other platforms/configurations.
var simdExpFloat32 func(dst, src []float32)
var simdExpFloat64 func(dst, src []float64)

// Declared here for other SIMD consumers.
var expFloat32x8 func(archsimd.Float32x8) archsimd.Float32x8
var expFloat64x4 func(archsimd.Float64x4) archsimd.Float64x4

// Per-vector exp implementations, set only when both AVX2 and FMA are available.
var expFloat32x8Constants *expConstantsFloat32x8
var expFloat64x4Constants *expConstantsFloat64x4

func init() {
	if archsimd.X86.AVX2() && archsimd.X86.FMA() {
		simdExpFloat32 = avx2ExpFloat32
		simdExpFloat64 = avx2ExpFloat64

		expFloat32x8 = avx2ExpFloat32x8
		expFloat64x4 = avx2ExpFloat64x4
		expFloat32x8Constants = newExpConstantsFloat32x8()
		expFloat64x4Constants = newExpConstantsFloat64x4()
	}
}

// avx2ExpFloat32 computes dst[i] = exp(src[i]) using AVX2 (256-bit, 8 float32/vector).
// Processes 8 elements per vector iteration with a scalar tail for the final 0-7 elements.
func avx2ExpFloat32(dst, src []float32) {
	n := len(src)
	i := 0

	for ; i+8 <= n; i += 8 {
		srcLoaded := archsimd.LoadFloat32x8Slice(src[i:])
		expLoaded := avx2ExpFloat32x8(srcLoaded)
		expLoaded.StoreSlice(dst[i:])
	}

	// scalar tail
	expScalar(dst[i:], src[i:])
}

// avx2ExpFloat64 computes dst[i] = exp(src[i]) using AVX2 (256-bit, 4 float64/vector).
// Processes 4 elements per vector iteration with a scalar tail for the final 0-3 elements.
func avx2ExpFloat64(dst, src []float64) {
	n := len(src)
	i := 0

	for ; i+4 <= n; i += 4 {
		srcLoaded := archsimd.LoadFloat64x4Slice(src[i:])
		expLoaded := avx2ExpFloat64x4(srcLoaded)
		expLoaded.StoreSlice(dst[i:])
	}

	// scalar tail
	expScalar(dst[i:], src[i:])
}

// Cephes-derived minimax polynomial coefficients for exp(r), r in
// [-ln2/2, ln2/2]. Same constants used by glibc/SLEEF-style expf.
const (
	// Upper bound of SIMD-safe range for exp(x) approximation.
	// Values above this are diverted to scalar/fallback handling.
	expClampHi = 88.37625885009765625
	// Lower bound of SIMD-safe range for exp(x) approximation.
	// Values below this threshold are diverted to scalar/fallback handling.
	expClampLo = -87.3365478515625
	// Bias on IEEE-754 float32 exponent bits.
	expBias = 127

	log2E = 1.44269504088896341 // log2(e)

	// ln2 split into hi/lo parts for precise range reduction.
	// ln2Hi is exactly representable in float32; ln2Lo corrects the error.
	ln2Hi = 0.693359375
	ln2Lo = -2.12194440e-4

	// Coefficients for the minimax polynomial P(r) approximating exp(r).
	c0 = 5.0000001201e-1
	c1 = 1.6666665459e-1
	c2 = 4.1665795894e-2
	c3 = 8.3334519073e-3
	c4 = 1.3981999507e-3
	c5 = 1.9875691500e-4
)

type expConstantsFloat32x8 struct {
	inf, negInf, nan, one, zero                           archsimd.Float32x8
	expBias                                               archsimd.Int32x8
	expClampHi, expClampLo, expOverflowHi, expUnderflowLo archsimd.Float32x8
	log2E, ln2Hi, ln2Lo                                   archsimd.Float32x8
	c0, c1, c2, c3, c4, c5                                archsimd.Float32x8
}

func newExpConstantsFloat32x8() *expConstantsFloat32x8 {
	return &expConstantsFloat32x8{
		inf:            archsimd.BroadcastFloat32x8(float32(math.Inf(1))),
		negInf:         archsimd.BroadcastFloat32x8(float32(math.Inf(-1))),
		nan:            archsimd.BroadcastFloat32x8(float32(math.NaN())),
		one:            archsimd.BroadcastFloat32x8(float32(1.0)),
		zero:           archsimd.BroadcastFloat32x8(float32(0.0)),
		expClampHi:     archsimd.BroadcastFloat32x8(float32(expClampHi)),
		expClampLo:     archsimd.BroadcastFloat32x8(float32(expClampLo)),
		expOverflowHi:  archsimd.BroadcastFloat32x8(float32(math.Log(math.MaxFloat32))),
		expUnderflowLo: archsimd.BroadcastFloat32x8(float32(math.Log(math.SmallestNonzeroFloat32))),
		expBias:        archsimd.BroadcastInt32x8(expBias),
		log2E:          archsimd.BroadcastFloat32x8(float32(log2E)),
		ln2Hi:          archsimd.BroadcastFloat32x8(float32(ln2Hi)),
		ln2Lo:          archsimd.BroadcastFloat32x8(float32(ln2Lo)),
		c0:             archsimd.BroadcastFloat32x8(float32(c0)),
		c1:             archsimd.BroadcastFloat32x8(float32(c1)),
		c2:             archsimd.BroadcastFloat32x8(float32(c2)),
		c3:             archsimd.BroadcastFloat32x8(float32(c3)),
		c4:             archsimd.BroadcastFloat32x8(float32(c4)),
		c5:             archsimd.BroadcastFloat32x8(float32(c5)),
	}
}

// avx2ExpFloat32x8 computes exp(x) for 8 float32 lanes in parallel using a single AVX2 register.
//
// The implementation follows a Cephes-derived minimax approximation (glibc/SLEEF-style expf)
// with SIMD-optimized range reduction and polynomial evaluation:
//
//  1. Fast SIMD path
//     For inputs in [expClampLo, expClampHi], the function performs fully vectorized
//     computation:
//     - Range reduction: x = n*ln2 + r, |r| ≤ ln2/2
//     - Polynomial approximation: exp(r) = P(r)
//     - Exponent reconstruction: 2^n via IEEE-754 bit manipulation
//     - Final composition: exp(x) = exp(r) * 2^n
//
//  2. Scalar fallback (edge stability domain)
//     For inputs outside the SIMD clamp range but still within finite overflow/underflow
//     bounds (expClampHi, log(MaxFloat32)] and [log(SmallestNonzeroFloat32), expClampLo) the function falls back to a scalar implementation.
//     Note: when ANY single lane falls in this edge domain, ALL 8 lanes are computed via the scalar fallback. The lower edge
//     domain spans [-103.3, -87.3] while the upper edge is only ~0.35 units ([88.376, 88.723]), typical ML values rarely hit this.
//
//  3. Inputs outside the finite exponent range are handled directly:
//     - x > log(MaxFloat32) returns +Inf (overflow)
//     - x < log(SmallestNonzeroFloat32) returns 0 (underflow)
//
//  4. Special value propagation: NaN and ±Inf inputs are handled explicitly.
func avx2ExpFloat32x8(x archsimd.Float32x8) archsimd.Float32x8 {
	c := expFloat32x8Constants

	// fall back to scalar if there are values between the clamp range and the overflow/underflow range
	upperEdgeMask := x.Greater(c.expClampHi).And(x.LessEqual(c.expOverflowHi))
	lowerEdgeMask := x.Less(c.expClampLo).And(x.GreaterEqual(c.expUnderflowLo))
	edgeMask := upperEdgeMask.Or(lowerEdgeMask)
	edgeMaskBits := edgeMask.ToBits()
	if edgeMaskBits > 0 {
		src := make([]float32, 8)
		dst := make([]float32, 8)
		x.StoreSlice(src)
		expScalar(dst, src)
		result := archsimd.LoadFloat32x8Slice(dst)
		return result
	}

	// mask special values before clamping
	nanMask := x.IsNaN()
	posInfMask := x.Equal(c.inf)
	negInfMask := x.Equal(c.negInf)

	// detect inputs that would overflow/underflow (before clamping)
	overflowMask := x.Greater(c.expOverflowHi)
	underflowMask := x.Less(c.expUnderflowLo)

	// clamp to avoid garbage in the exponent reconstruction for extreme values
	xClamped := x.Min(c.expClampHi)
	xClamped = xClamped.Max(c.expClampLo)

	// range reduction: x = n*ln2 + r, |r| <= ln2/2
	n := xClamped.Mul(c.log2E).RoundToEven()

	// r = x - n*ln2Hi - n*ln2Lo  (two-step subtraction for precision)
	r := xClamped.Sub(n.Mul(c.ln2Hi))
	r = r.Sub(n.Mul(c.ln2Lo))

	// Compute P(r) = c0 + c1*r + c2*(r^2) + c3*(r^3) + c4*(r^4) + c5*(r^5)
	// using Horner's method.
	p := c.c5
	p = p.MulAdd(r, c.c4)
	p = p.MulAdd(r, c.c3)
	p = p.MulAdd(r, c.c2)
	p = p.MulAdd(r, c.c1)
	p = p.MulAdd(r, c.c0) // P(r)

	// exp(r) = 1 + r + (r^2)*P(r)
	onePlusR := r.Add(c.one)
	r2 := r.Mul(r)
	expR := r2.MulAdd(p, onePlusR)

	// Reconstruct 2^n via exponent bits.
	// IEEE-754 float32: biased exponent = n + 127, stored in bits 23..30.
	// Shifting (n + 127) << 23 gives the bit representation of 2^n (sign=0, mantissa=0).
	nInt32 := n.ConvertToInt32()
	biased := nInt32.Add(c.expBias)
	pow2n := biased.ShiftAllLeft(23).AsFloat32x8()

	result := expR.Mul(pow2n) // exp(x) = exp(r) * 2^n

	// restore overflow/underflow to match math.Exp behavior
	result = c.inf.Merge(result, overflowMask)   // exp(x > expHi) = +Inf
	result = c.zero.Merge(result, underflowMask) // exp(x < expLo) = 0

	// restore special values
	result = c.zero.Merge(result, negInfMask) // exp(-Inf) = 0
	result = c.inf.Merge(result, posInfMask)  // exp(+Inf) = +Inf
	result = c.nan.Merge(result, nanMask)     // exp(NaN) = NaN

	return result
}

// Go math.Exp rational approximation coefficients for exp(r) (float64).
const (
	expHiF64   = 709.782712893384  // clamp: exp(x) overflows float64 above this
	expLoF64   = -708.396418532264 // clamp: exp(x) underflows to 0 below this
	expBiasF64 = 1023

	log2EF64 = 1.44269504088896341 // log2(e)

	// ln2 split into hi/lo for precise range reduction.
	// ln2Hi is exactly representable in float64; ln2Lo corrects the error.
	ln2HiF64 = 0.693145751953125
	ln2LoF64 = 1.42860682030941723212e-6

	// Coefficients for the polynomial R(r^2) = 2 + P1*r^2 + P2*r^4 + ... + P5*r^10
	// exp(r) = 1 + r + r*c/(2-c) where c = r - (r^2)*R1(r^2) and R1 = P1 + P2*z + ...
	p1 = 1.66666666666666657415e-01
	p2 = -2.77777777770155933842e-03
	p3 = 6.61375632143793436117e-05
	p4 = -1.65339022054652515390e-06
	p5 = 4.13813679705723846039e-08
)

type expConstantsFloat64x4 struct {
	inf, negInf, nan, one, two, zero  archsimd.Float64x4
	expBias                           archsimd.Int64x4
	expHi, expLo, log2E, ln2Hi, ln2Lo archsimd.Float64x4
	p1, p2, p3, p4, p5                archsimd.Float64x4
}

func newExpConstantsFloat64x4() *expConstantsFloat64x4 {
	return &expConstantsFloat64x4{
		inf:     archsimd.BroadcastFloat64x4(math.Inf(1)),
		negInf:  archsimd.BroadcastFloat64x4(math.Inf(-1)),
		nan:     archsimd.BroadcastFloat64x4(math.NaN()),
		one:     archsimd.BroadcastFloat64x4(1.0),
		two:     archsimd.BroadcastFloat64x4(2.0),
		zero:    archsimd.BroadcastFloat64x4(0.0),
		expHi:   archsimd.BroadcastFloat64x4(expHiF64),
		expLo:   archsimd.BroadcastFloat64x4(expLoF64),
		expBias: archsimd.BroadcastInt64x4(expBiasF64),
		log2E:   archsimd.BroadcastFloat64x4(log2EF64),
		ln2Hi:   archsimd.BroadcastFloat64x4(ln2HiF64),
		ln2Lo:   archsimd.BroadcastFloat64x4(ln2LoF64),
		p1:      archsimd.BroadcastFloat64x4(p1),
		p2:      archsimd.BroadcastFloat64x4(p2),
		p3:      archsimd.BroadcastFloat64x4(p3),
		p4:      archsimd.BroadcastFloat64x4(p4),
		p5:      archsimd.BroadcastFloat64x4(p5),
	}
}

// avx2ExpFloat64x4 computes exp(x) for 4 float64 lanes in parallel (one AVX2 register).
//
// Uses the same rational approximation as Go's math.Exp:
//
//	R(z) = 2 + P1*z + P2*(z^2) + P3*(z^3) + P4*(z^4) + P5*(z^5),  z = r*r
//	R1(r) = r - z*(P1 + z*(P2 + z*(P3 + z*(P4 + z*P5))))
//	exp(r) = 1 + r + r*R1(r) / (2 - R1(r))
func avx2ExpFloat64x4(x archsimd.Float64x4) archsimd.Float64x4 {
	c := expFloat64x4Constants
	// mask special values before clamping
	nanMask := x.IsNaN()
	posInfMask := x.Equal(c.inf)
	negInfMask := x.Equal(c.negInf)

	// detect inputs that would overflow/underflow (before clamping)
	overflowMask := x.Greater(c.expHi)
	underflowMask := x.Less(c.expLo)

	// clamp to avoid garbage in the exponent reconstruction for extreme values
	xClamped := x.Min(c.expHi)
	xClamped = xClamped.Max(c.expLo)

	// range reduction: x = n*ln2 + r, |r| <= ln2/2
	n := xClamped.Mul(c.log2E).RoundToEven()

	// r = x - n*ln2Hi - n*ln2Lo  (two-step subtraction for precision)
	r := xClamped.Sub(n.Mul(c.ln2Hi))
	r = r.Sub(n.Mul(c.ln2Lo))

	// Compute R1(r) = r - (r^2)*(P1 + (r^2)*(P2 + (r^2)*(P3 + (r^2)*(P4 + (r^2)*P5))))
	// using Horner's method in z = (r^2).

	z := r.Mul(r)

	poly := c.p5
	poly = poly.MulAdd(z, c.p4)
	poly = poly.MulAdd(z, c.p3)
	poly = poly.MulAdd(z, c.p2)
	poly = poly.MulAdd(z, c.p1) // poly = P1 + P2*z + P3*(z^2) + P4*(z^3) + P5*(z^4)

	r1 := z.Mul(poly) // z * (P1 + P2*z + ...)
	r1 = r.Sub(r1)    // R1(r) = r - z*(P1 + P2*z + ...)

	// exp(r) = 1 + r + r*R1(r) / (2 - R1(r))
	denom := c.two.Sub(r1)
	frac := r.Mul(r1).Div(denom) // r*R1/(2 - R1)
	expR := c.one.Add(r.Add(frac))

	// Reconstruct 2^n via exponent bits.
	// IEEE-754 float64: biased exponent = n + 1023, stored in bits 52..62.
	// Shifting (n + 1023) << 52 gives the bit representation of 2^n (sign=0, mantissa=0).
	// ConvertToInt64 is AVX512-only; use ConvertToInt32 and sign-extend (n fits in int32).
	nInt32 := n.ConvertToInt32()
	nInt64 := nInt32.ExtendToInt64()
	biased := nInt64.Add(c.expBias)
	pow2n := biased.ShiftAllLeft(52).AsFloat64x4()

	result := expR.Mul(pow2n) // exp(x) = exp(r) * 2^n

	// restore overflow/underflow to match math.Exp behavior
	result = c.inf.Merge(result, overflowMask)   // exp(x > expHi) = +Inf
	result = c.zero.Merge(result, underflowMask) // exp(x < expLo) = 0

	// restore special values
	result = c.zero.Merge(result, negInfMask) // exp(-Inf) = 0
	result = c.inf.Merge(result, posInfMask)  // exp(+Inf) = +Inf
	result = c.nan.Merge(result, nanMask)     // exp(NaN) = NaN

	return result
}
