// These fuzz tests ensure element-wise exp produces identical results when
// performed with or without a SIMD kernel.
//
// Run fuzz tests with:
//
//	go test -fuzz=FuzzExpFloat32 -fuzztime=60s
// 	go test -fuzz=FuzzExpFloat64 -fuzztime=60s

package cpu

import (
	"encoding/binary"
	"math"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tolerance"
)

// expF32SeedOptions is a set of "interesting" float32 values for fuzzing exp().
//
// The list is biased toward values that exercise range reduction,
// polynomial approximation, overflow/underflow transitions, IEEE-754
// edge cases, and denormal handling.
var expF32SeedOptions = func() []float32 {
	ln2 := float32(math.Ln2)

	maxLog := float32(math.Log(float64(math.MaxFloat32)))
	minSubnormalLog := float32(math.Log(float64(math.SmallestNonzeroFloat32)))

	return []float32{
		// Signed zeros.
		0.0,
		float32(math.Copysign(0.0, -1)),

		// Tiny values around zero.
		math.SmallestNonzeroFloat32,
		-math.SmallestNonzeroFloat32,
		math.Float32frombits(0x00000002),
		-math.Float32frombits(0x00000002),
		math.Float32frombits(0x00000004),
		-math.Float32frombits(0x00000004),
		1e-9,
		-1e-9,

		// Around +/-1.
		math.Nextafter32(1.0, 0.0),
		-math.Nextafter32(1.0, 0.0),
		1.0,
		-1.0,
		math.Nextafter32(1.0, float32(math.Inf(1))),
		-math.Nextafter32(1.0, float32(math.Inf(1))),

		// Around +/-ln(2), where range reduction often changes.
		math.Nextafter32(ln2, 0),
		ln2,
		math.Nextafter32(ln2, float32(math.Inf(1))),
		math.Nextafter32(-ln2, float32(math.Inf(-1))),
		-ln2,
		math.Nextafter32(-ln2, 0),

		// Additional range-reduction stress.
		2 * ln2,
		-2 * ln2,
		10 * ln2,
		-10 * ln2,

		// Polynomial approximation stress.
		-0.5,
		-0.25,
		-0.125,
		0.125,
		0.25,
		0.5,

		// Overflow boundary: exp(x) = +Inf.
		math.Nextafter32(maxLog, 0),
		maxLog,
		math.Nextafter32(maxLog, float32(math.Inf(1))),

		// Underflow boundary: exp(x) = smallest subnormal / 0.
		math.Nextafter32(minSubnormalLog, float32(math.Inf(-1))),
		minSubnormalLog,
		math.Nextafter32(minSubnormalLog, 0),

		// IEEE-754 special values.
		float32(math.Inf(1)),
		float32(math.Inf(-1)),
		float32(math.NaN()),
		math.Float32frombits(0x7fc00001), // quiet NaN
		math.Float32frombits(0x7fffffff), // NaN with all payload bits set
		math.Float32frombits(0xffc00001), // negative NaN

		// Subnormal/normal boundaries.
		math.Float32frombits(0x007fffff), // largest subnormal
		-math.Float32frombits(0x007fffff),
		math.Float32frombits(0x00800000), // smallest normal
		-math.Float32frombits(0x00800000),

		// Largest finite values.
		math.MaxFloat32,
		-math.MaxFloat32,
	}
}

// makeExpFloat32SeedCorpus constructs a byte array of n float32 values. It populates the
// array by selecting values randomly from expF32SeedOptions().
func makeExpFloat32SeedCorpus(n int) []byte {
	bytesCorpus := make([]byte, n*4)

	seedOpts := expF32SeedOptions()

	for i := range n {
		seedVal := seedOpts[rand.Int()%len(seedOpts)]
		binary.LittleEndian.PutUint32(bytesCorpus[i*4:], math.Float32bits(seedVal))
	}
	return bytesCorpus
}

// FuzzExpFloat32 verifies that the SIMD exp
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzExpFloat32(f *testing.F) {
	if simdExpFloat32 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float32]()

	for _, n := range simdTestSliceLengths {
		srcSeed := makeExpFloat32SeedCorpus(n)
		f.Add(srcSeed)
	}

	f.Fuzz(func(t *testing.T, srcBytes []byte) {
		n := len(srcBytes) / 4

		src := make([]float32, n)
		dstScalar := make([]float32, n)
		dstSIMD := make([]float32, n)

		for i := range src {
			src[i] = math.Float32frombits(binary.LittleEndian.Uint32(srcBytes[i*4:]))
		}

		expScalar(dstScalar, src)
		simdExpFloat32(dstSIMD, src)

		if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
			t.Errorf("src=%f", src)
			t.Fatal(err)
		}
	})
}

// expF64SeedOptions is a set of "interesting" values to be picked
// randomly when generating a fuzzing seed corpus.
//
// The list is biased toward values that exercise range reduction,
// polynomial approximation, overflow/underflow transitions, IEEE-754
// edge cases, and denormal handling.
var expF64SeedOptions = func() []float64 {
	ln2 := math.Ln2

	maxLog := math.Log(math.MaxFloat64)
	minSubnormalLog := math.Log(math.SmallestNonzeroFloat64)

	return []float64{
		// Signed zeros.
		0.0,
		math.Copysign(0.0, -1),

		// Tiny values around zero (float64 denormals).
		math.SmallestNonzeroFloat64,
		-math.SmallestNonzeroFloat64,
		math.Float64frombits(0x0000000000000002),
		-math.Float64frombits(0x0000000000000002),
		math.Float64frombits(0x0000000000000004),
		-math.Float64frombits(0x0000000000000004),
		1e-18,
		-1e-18,

		// Around +/-1.
		math.Nextafter(1.0, 0.0),
		-math.Nextafter(1.0, 0.0),
		1.0,
		-1.0,
		math.Nextafter(1.0, math.Inf(1)),
		-math.Nextafter(1.0, math.Inf(1)),

		// Around +/-ln(2), key range-reduction boundary.
		math.Nextafter(ln2, 0),
		ln2,
		math.Nextafter(ln2, math.Inf(1)),
		math.Nextafter(-ln2, math.Inf(-1)),
		-ln2,
		math.Nextafter(-ln2, 0),

		// Additional range reduction stress.
		2 * ln2,
		-2 * ln2,
		10 * ln2,
		-10 * ln2,
		20 * ln2,
		-20 * ln2,

		// Polynomial approximation region stress.
		-0.5,
		-0.25,
		-0.125,
		0.125,
		0.25,
		0.5,

		// Overflow boundary: exp(x) = +Inf.
		math.Nextafter(maxLog, 0),
		maxLog,
		math.Nextafter(maxLog, math.Inf(1)),

		// Underflow boundary: exp(x) = -Inf.
		math.Nextafter(minSubnormalLog, math.Inf(-1)),
		minSubnormalLog,
		math.Nextafter(minSubnormalLog, 0),

		// IEEE special values.
		math.Inf(1),
		math.Inf(-1),
		math.NaN(),
		math.Float64frombits(0x7ff8000000000001), // quiet NaN
		math.Float64frombits(0x7fffffffffffffff), // NaN payload max
		math.Float64frombits(0xfff8000000000001), // negative NaN

		// Subnormal/normal boundary.
		math.Float64frombits(0x000fffffffffffff), // largest subnormal
		-math.Float64frombits(0x000fffffffffffff),
		math.Float64frombits(0x0010000000000000), // smallest normal
		-math.Float64frombits(0x0010000000000000),

		// Largest finite values.
		math.MaxFloat64,
		-math.MaxFloat64,
	}
}

// makeExpFloat64SeedCorpus constructs a byte array of n float64 values. It populates the
// array by selecting values randomly from expF64SeedOptions.
func makeExpFloat64SeedCorpus(n int) []byte {
	bytesCorpus := make([]byte, n*8)

	seedOpts := expF64SeedOptions()

	for i := range n {
		seedVal := seedOpts[rand.Int()%len(seedOpts)]
		binary.LittleEndian.PutUint64(bytesCorpus[i*8:], math.Float64bits(seedVal))
	}
	return bytesCorpus
}

// FuzzExpFloat64 verifies that the SIMD exp
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzExpFloat64(f *testing.F) {
	if simdExpFloat64 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float64]()

	for _, n := range simdTestSliceLengths {
		srcSeed := makeExpFloat64SeedCorpus(n)
		f.Add(srcSeed)
	}

	f.Fuzz(func(t *testing.T, srcBytes []byte) {
		n := len(srcBytes) / 8

		src := make([]float64, n)
		dstScalar := make([]float64, n)
		dstSIMD := make([]float64, n)

		for i := range src {
			src[i] = math.Float64frombits(binary.LittleEndian.Uint64(srcBytes[i*8:]))
		}

		expScalar(dstScalar, src)
		simdExpFloat64(dstSIMD, src)

		if err := tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol); err != nil {
			t.Fatal(err)
		}
	})
}
