// These fuzz tests ensure element-wise arithmetic ops produce identical results when
// performed with or without a SIMD kernel.
//
// Run fuzz tests with:
//
//	go test -fuzz=FuzzAddInplaceFloat32 -fuzztime=60s
// 	go test -fuzz=FuzzSubInplaceFloat32 -fuzztime=60s
//  go test -fuzz=FuzzMulInplaceFloat32 -fuzztime=60s
//  go test -fuzz=FuzzDivInplaceFloat32 -fuzztime=60s
//
//	go test -fuzz=FuzzAddInplaceFloat64 -fuzztime=60s
//	go test -fuzz=FuzzSubInplaceFloat64 -fuzztime=60s
//	go test -fuzz=FuzzMulInplaceFloat64 -fuzztime=60s
//	go test -fuzz=FuzzDivInplaceFloat64 -fuzztime=60s
//
//	go test -fuzz=FuzzAddInplaceInt32 -fuzztime=60s
//	go test -fuzz=FuzzSubInplaceInt32 -fuzztime=60s
//	go test -fuzz=FuzzMulInplaceInt32 -fuzztime=60s
//
//	go test -fuzz=FuzzAddInplaceInt64 -fuzztime=60s
//	go test -fuzz=FuzzSubInplaceInt64 -fuzztime=60s
//	go test -fuzz=FuzzMulInplaceInt64 -fuzztime=60s

package cpu

import (
	"encoding/binary"
	"math"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tolerance"
)

// f32SeedOptions is a set of "interesting" values to be picked
// randomly when generating a fuzzing seed corpus.
var f32SeedOptions = []float32{
	0.0,
	float32(math.Copysign(0.0, -1)), // -0.0
	1e-9,
	-1e-9,
	math.Float32frombits(0x3f7fffff), // next representable below 1.0
	-math.Float32frombits(0x3f7fffff),
	1.0,
	-1.0,
	math.Float32frombits(0x3f800001), // next representable above 1.0
	-math.Float32frombits(0x3f800001),
	1e5,
	-1e5,
	float32(math.NaN()),
	float32(math.Inf(1)),
	float32(math.Inf(-1)),
	math.SmallestNonzeroFloat32, // minimum subnormal
	-math.SmallestNonzeroFloat32,
	math.Float32frombits(0x007fffff), // maximum subnormal
	-math.Float32frombits(0x007fffff),
	math.Float32frombits(0x00800000), // minimum normal
	-math.Float32frombits(0x00800000),
	math.MaxFloat32, // maximum normal
	-math.MaxFloat32,
}

// makeFloat32SeedCorpus constructs a byte array of n float32 values. It populates the
// array by selecting values randomly from f32SeedOptions.
func makeFloat32SeedCorpus(n int) []byte {
	bytesCorpus := make([]byte, n*4)

	for i := range n {
		seedVal := f32SeedOptions[rand.Int()%len(f32SeedOptions)]
		binary.LittleEndian.PutUint32(bytesCorpus[i*4:], math.Float32bits(seedVal))
	}
	return bytesCorpus
}

// FuzzAddInplaceFloat32 verifies that the SIMD add-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzAddInplaceFloat32(f *testing.F) {
	if simdAddInplaceFloat32 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float32]()

	for _, n := range simdTestSliceLengths {
		aSeed := makeFloat32SeedCorpus(n)
		bSeed := makeFloat32SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}

	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 4
		if m := len(bBytes) / 4; m < n {
			n = m
		}

		a := make([]float32, n)
		b := make([]float32, n)

		for i := range a {
			a[i] = math.Float32frombits(binary.LittleEndian.Uint32(aBytes[i*4:]))
			b[i] = math.Float32frombits(binary.LittleEndian.Uint32(bBytes[i*4:]))
		}

		if err := inplaceSIMDMatchesScalarFloat(a, b, &simdAddInplaceFloat32, addInplaceFloat32, tol); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzSubInplaceFloat32 verifies that the SIMD sub-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzSubInplaceFloat32(f *testing.F) {
	if simdSubInplaceFloat32 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float32]()

	for _, n := range simdTestSliceLengths {
		aSeed := makeFloat32SeedCorpus(n)
		bSeed := makeFloat32SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 4
		if m := len(bBytes) / 4; m < n {
			n = m
		}

		a := make([]float32, n)
		b := make([]float32, n)

		for i := range a {
			a[i] = math.Float32frombits(binary.LittleEndian.Uint32(aBytes[i*4:]))
			b[i] = math.Float32frombits(binary.LittleEndian.Uint32(bBytes[i*4:]))
		}

		if err := inplaceSIMDMatchesScalarFloat(a, b, &simdSubInplaceFloat32, subInplaceFloat32, tol); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzMulInplaceFloat32 verifies that the SIMD mul-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzMulInplaceFloat32(f *testing.F) {
	if simdMulInplaceFloat32 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float32]()

	for _, n := range simdTestSliceLengths {
		aSeed := makeFloat32SeedCorpus(n)
		bSeed := makeFloat32SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 4
		if m := len(bBytes) / 4; m < n {
			n = m
		}

		a := make([]float32, n)
		b := make([]float32, n)

		for i := range a {
			a[i] = math.Float32frombits(binary.LittleEndian.Uint32(aBytes[i*4:]))
			b[i] = math.Float32frombits(binary.LittleEndian.Uint32(bBytes[i*4:]))
		}

		if err := inplaceSIMDMatchesScalarFloat(a, b, &simdMulInplaceFloat32, mulInplaceFloat32, tol); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzDivInplaceFloat32 verifies that the SIMD div-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzDivInplaceFloat32(f *testing.F) {
	if simdDivInplaceFloat32 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float32]()

	for _, n := range simdTestSliceLengths {
		aSeed := makeFloat32SeedCorpus(n)
		bSeed := makeFloat32SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 4
		if m := len(bBytes) / 4; m < n {
			n = m
		}

		a := make([]float32, n)
		b := make([]float32, n)

		for i := range a {
			a[i] = math.Float32frombits(binary.LittleEndian.Uint32(aBytes[i*4:]))
			b[i] = math.Float32frombits(binary.LittleEndian.Uint32(bBytes[i*4:]))
		}

		if err := inplaceSIMDMatchesScalarFloat(a, b, &simdDivInplaceFloat32, divInplaceFloat32, tol); err != nil {
			t.Fatal(err)
		}
	})
}

// f64SeedOptions is a set of "interesting" values to be picked
// randomly when generating a fuzzing seed corpus.
var f64SeedOptions = []float64{
	0.0,
	math.Copysign(0.0, -1), // -0.0
	1e-9,
	-1e-9,
	math.Float64frombits(0x3fefffffffffffff), // next representable below 1.0
	-math.Float64frombits(0x3fefffffffffffff),
	1.0,
	-1.0,
	math.Float64frombits(0x3ff0000000000001), // next representable above 1.0
	-math.Float64frombits(0x3ff0000000000001),
	1e5,
	-1e5,
	math.NaN(),
	math.Inf(1),
	math.Inf(-1),
	math.SmallestNonzeroFloat64, // minimum subnormal
	-math.SmallestNonzeroFloat64,
	math.Float64frombits(0x000fffffffffffff), // maximum subnormal
	-math.Float64frombits(0x000fffffffffffff),
	math.Float64frombits(0x0010000000000000), // minimum normal
	-math.Float64frombits(0x0010000000000000),
	math.MaxFloat64, // maximum finite
	-math.MaxFloat64,
}

// makeFloat64SeedCorpus constructs a byte array of n float64 values. It populates the
// array by selecting values randomly from f64SeedOptions.
func makeFloat64SeedCorpus(n int) []byte {
	bytesCorpus := make([]byte, n*8)

	for i := range n {
		seedVal := f64SeedOptions[rand.Int()%len(f64SeedOptions)]
		binary.LittleEndian.PutUint64(bytesCorpus[i*8:], math.Float64bits(seedVal))
	}
	return bytesCorpus
}

// FuzzAddInplaceFloat64 verifies that the SIMD add-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzAddInplaceFloat64(f *testing.F) {
	if simdAddInplaceFloat64 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float64]()

	for _, n := range simdTestSliceLengths {
		aSeed := makeFloat64SeedCorpus(n)
		bSeed := makeFloat64SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 8
		if m := len(bBytes) / 8; m < n {
			n = m
		}

		a := make([]float64, n)
		b := make([]float64, n)

		for i := range a {
			a[i] = math.Float64frombits(binary.LittleEndian.Uint64(aBytes[i*8:]))
			b[i] = math.Float64frombits(binary.LittleEndian.Uint64(bBytes[i*8:]))
		}

		if err := inplaceSIMDMatchesScalarFloat(a, b, &simdAddInplaceFloat64, addInplaceFloat64, tol); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzSubInplaceFloat64 verifies that the SIMD sub-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzSubInplaceFloat64(f *testing.F) {
	if simdSubInplaceFloat64 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float64]()

	for _, n := range simdTestSliceLengths {
		aSeed := makeFloat64SeedCorpus(n)
		bSeed := makeFloat64SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 8
		if m := len(bBytes) / 8; m < n {
			n = m
		}

		a := make([]float64, n)
		b := make([]float64, n)

		for i := range a {
			a[i] = math.Float64frombits(binary.LittleEndian.Uint64(aBytes[i*8:]))
			b[i] = math.Float64frombits(binary.LittleEndian.Uint64(bBytes[i*8:]))
		}

		if err := inplaceSIMDMatchesScalarFloat(a, b, &simdSubInplaceFloat64, subInplaceFloat64, tol); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzMulInplaceFloat64 verifies that the SIMD mul-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzMulInplaceFloat64(f *testing.F) {
	if simdMulInplaceFloat64 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float64]()

	for _, n := range simdTestSliceLengths {
		aSeed := makeFloat64SeedCorpus(n)
		bSeed := makeFloat64SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 8
		if m := len(bBytes) / 8; m < n {
			n = m
		}

		a := make([]float64, n)
		b := make([]float64, n)

		for i := range a {
			a[i] = math.Float64frombits(binary.LittleEndian.Uint64(aBytes[i*8:]))
			b[i] = math.Float64frombits(binary.LittleEndian.Uint64(bBytes[i*8:]))
		}

		if err := inplaceSIMDMatchesScalarFloat(a, b, &simdMulInplaceFloat64, mulInplaceFloat64, tol); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzDivInplaceFloat64 verifies that the SIMD div-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzDivInplaceFloat64(f *testing.F) {
	if simdDivInplaceFloat64 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	tol := tolerance.NewDefaultTolerance[float64]()

	for _, n := range simdTestSliceLengths {
		aSeed := makeFloat64SeedCorpus(n)
		bSeed := makeFloat64SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 8
		if m := len(bBytes) / 8; m < n {
			n = m
		}

		a := make([]float64, n)
		b := make([]float64, n)

		for i := range a {
			a[i] = math.Float64frombits(binary.LittleEndian.Uint64(aBytes[i*8:]))
			b[i] = math.Float64frombits(binary.LittleEndian.Uint64(bBytes[i*8:]))
		}

		if err := inplaceSIMDMatchesScalarFloat(a, b, &simdDivInplaceFloat64, divInplaceFloat64, tol); err != nil {
			t.Fatal(err)
		}
	})
}

// i32SeedOptions is a set of "interesting" values to be picked
// randomly when generating a fuzzing seed corpus.
var i32SeedOptions = []int32{
	0,
	1,
	-1,
	(1 << 8),
	-(1 << 8),
	(1 << 8) - 1,
	-(1 << 8) + 1,
	(1 << 15),
	-(1 << 15),
	(1 << 15) - 1,
	-(1 << 15) + 1,
	(1 << 30),
	-(1 << 30),
	(1 << 30) - 1,
	-(1 << 30) + 1,
	math.MaxInt32 - 1,
	math.MaxInt32,
	math.MinInt32,
	math.MinInt32 + 1,
}

// makeInt32SeedCorpus constructs a byte array of n int32 values. It populates the
// array by selecting values randomly from i32SeedOptions.
func makeInt32SeedCorpus(n int) []byte {
	bytesCorpus := make([]byte, n*4)

	for i := range n {
		seedVal := i32SeedOptions[rand.Int()%len(i32SeedOptions)]
		binary.LittleEndian.PutUint32(bytesCorpus[i*4:], uint32(seedVal))
	}
	return bytesCorpus
}

// FuzzAddInplaceInt32 verifies that the SIMD add-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzAddInplaceInt32(f *testing.F) {
	if simdAddInplaceInt32 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, n := range simdTestSliceLengths {
		aSeed := makeInt32SeedCorpus(n)
		bSeed := makeInt32SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 4
		if m := len(bBytes) / 4; m < n {
			n = m
		}

		a := make([]int32, n)
		b := make([]int32, n)

		for i := range a {
			a[i] = int32(binary.LittleEndian.Uint32(aBytes[i*4:]))
			b[i] = int32(binary.LittleEndian.Uint32(bBytes[i*4:]))
		}

		if err := inplaceSIMDMatchesScalarInt(a, b, &simdAddInplaceInt32, addInplaceInt32); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzSubInplaceInt32 verifies that the SIMD sub-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzSubInplaceInt32(f *testing.F) {
	if simdSubInplaceInt32 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, n := range simdTestSliceLengths {
		aSeed := makeInt32SeedCorpus(n)
		bSeed := makeInt32SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 4
		if m := len(bBytes) / 4; m < n {
			n = m
		}

		a := make([]int32, n)
		b := make([]int32, n)

		for i := range a {
			a[i] = int32(binary.LittleEndian.Uint32(aBytes[i*4:]))
			b[i] = int32(binary.LittleEndian.Uint32(bBytes[i*4:]))
		}

		if err := inplaceSIMDMatchesScalarInt(a, b, &simdSubInplaceInt32, subInplaceInt32); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzMulInplaceInt32 verifies that the SIMD mul-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzMulInplaceInt32(f *testing.F) {
	if simdMulInplaceInt32 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, n := range simdTestSliceLengths {
		aSeed := makeInt32SeedCorpus(n)
		bSeed := makeInt32SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 4
		if m := len(bBytes) / 4; m < n {
			n = m
		}

		a := make([]int32, n)
		b := make([]int32, n)

		for i := range a {
			a[i] = int32(binary.LittleEndian.Uint32(aBytes[i*4:]))
			b[i] = int32(binary.LittleEndian.Uint32(bBytes[i*4:]))
		}

		if err := inplaceSIMDMatchesScalarInt(a, b, &simdMulInplaceInt32, mulInplaceInt32); err != nil {
			t.Fatal(err)
		}
	})
}

// i64SeedOptions is a set of "interesting" values to be picked
// randomly when generating a fuzzing seed corpus.
var i64SeedOptions = []int64{
	0,
	1,
	-1,
	(1 << 8),
	-(1 << 8),
	(1 << 8) - 1,
	-(1 << 8) + 1,
	(1 << 15),
	-(1 << 15),
	(1 << 15) - 1,
	-(1 << 15) + 1,
	(1 << 30),
	-(1 << 30),
	(1 << 30) - 1,
	-(1 << 30) + 1,
	(1 << 62),
	-(1 << 62),
	(1 << 62) - 1,
	-(1 << 62) + 1,
	math.MaxInt64 - 1,
	math.MaxInt64,
	math.MinInt64,
	math.MinInt64 + 1,
}

// makeInt64SeedCorpus constructs a byte array of n int64 values. It populates the
// array by selecting values randomly from i64SeedOptions.
func makeInt64SeedCorpus(n int) []byte {
	bytesCorpus := make([]byte, n*8)

	for i := range n {
		seedVal := i64SeedOptions[rand.Int()%len(i64SeedOptions)]
		binary.LittleEndian.PutUint64(bytesCorpus[i*8:], uint64(seedVal))
	}
	return bytesCorpus
}

// FuzzAddInplaceInt64 verifies that the SIMD add-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzAddInplaceInt64(f *testing.F) {
	if simdAddInplaceInt64 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, n := range simdTestSliceLengths {
		aSeed := makeInt64SeedCorpus(n)
		bSeed := makeInt64SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 8
		if m := len(bBytes) / 8; m < n {
			n = m
		}

		a := make([]int64, n)
		b := make([]int64, n)

		for i := range a {
			a[i] = int64(binary.LittleEndian.Uint64(aBytes[i*8:]))
			b[i] = int64(binary.LittleEndian.Uint64(bBytes[i*8:]))
		}

		if err := inplaceSIMDMatchesScalarInt(a, b, &simdAddInplaceInt64, addInplaceInt64); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzSubInplaceInt64 verifies that the SIMD sub-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzSubInplaceInt64(f *testing.F) {
	if simdSubInplaceInt64 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, n := range simdTestSliceLengths {
		aSeed := makeInt64SeedCorpus(n)
		bSeed := makeInt64SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 8
		if m := len(bBytes) / 8; m < n {
			n = m
		}

		a := make([]int64, n)
		b := make([]int64, n)

		for i := range a {
			a[i] = int64(binary.LittleEndian.Uint64(aBytes[i*8:]))
			b[i] = int64(binary.LittleEndian.Uint64(bBytes[i*8:]))
		}

		if err := inplaceSIMDMatchesScalarInt(a, b, &simdSubInplaceInt64, subInplaceInt64); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzMulInplaceInt64 verifies that the SIMD mul-inplace
// kernel produces results matching the scalar fallback with fuzzed input slices.
func FuzzMulInplaceInt64(f *testing.F) {
	if simdMulInplaceInt64 == nil {
		f.Skip("SIMD implementation not available (build without GOEXPERIMENT=simd or non-amd64)")
	}

	for _, n := range simdTestSliceLengths {
		aSeed := makeInt64SeedCorpus(n)
		bSeed := makeInt64SeedCorpus(n)
		f.Add(aSeed, bSeed)
	}
	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		n := len(aBytes) / 8
		if m := len(bBytes) / 8; m < n {
			n = m
		}

		a := make([]int64, n)
		b := make([]int64, n)

		for i := range a {
			a[i] = int64(binary.LittleEndian.Uint64(aBytes[i*8:]))
			b[i] = int64(binary.LittleEndian.Uint64(bBytes[i*8:]))
		}

		if err := inplaceSIMDMatchesScalarInt(a, b, &simdMulInplaceInt64, mulInplaceInt64); err != nil {
			t.Fatal(err)
		}
	})
}
