package cpu

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tolerance"
)

// simdTestSliceLengths is a set of slice lengths to test element-wise SIMD operations on.
var simdTestSliceLengths = []int{1, 3, 4, 7, 8, 13, 16, 31, 32, 64, 100, 128, 256, 511, 1024}

// InplaceOpCase is a struct to facilitate table-driven SIMD tests on inplace element-wise operations.
type InplaceOpCase[T float32 | float64 | int32 | int64] struct {
	name       string
	kernel     *func(a, b []T)
	op         func(a, b []T)
	aGenerator func(*rand.Rand) T
	bGenerator func(*rand.Rand) T
}

// VectorizedOpCase is a struct to facilitate table-driven SIMD tests on vectorized element-wise operations.
type VectorizedOpCase[T float32 | float64 | int32 | int64] struct {
	name       string
	kernel     *func(dst, a, b []T)
	op         func(dst, a, b []T)
	aGenerator func(*rand.Rand) T
	bGenerator func(*rand.Rand) T
}

// inplaceSIMDMatchesScalarFloat performs an inplace element-wise arithmetic operation with and without a SIMD kernel.
// Returns an error if the results differ.
func inplaceSIMDMatchesScalarFloat[T float32 | float64](aSlice, bSlice []T, simdKernel *func(a, b []T), inplaceOp func(a, b []T), tol *tolerance.Tolerance[T]) error {
	aScalar := make([]T, len(aSlice))
	copy(aScalar, aSlice)
	saved := *simdKernel
	*simdKernel = nil
	inplaceOp(aScalar, bSlice)
	*simdKernel = saved

	inplaceOp(aSlice, bSlice)

	return tolerance.AssertAllApproxEqual(aScalar, aSlice, tol)
}

// inplaceSIMDMatchesScalarInt performs an inplace element-wise arithmetic operation with and without a SIMD kernel.
// Returns an error if the results differ.
func inplaceSIMDMatchesScalarInt[T int32 | int64](aSlice, bSlice []T, simdKernel *func(a, b []T), inplaceOp func(a, b []T)) error {
	aScalar := make([]T, len(aSlice))
	copy(aScalar, aSlice)
	saved := *simdKernel
	*simdKernel = nil
	inplaceOp(aScalar, bSlice)
	*simdKernel = saved

	inplaceOp(aSlice, bSlice)

	for i := range aScalar {
		if aScalar[i] != aSlice[i] {
			return fmt.Errorf("element[%d]: SIMD=%d scalar=%d", i, aSlice[i], aScalar[i])
		}
	}
	return nil
}

// vectorizedSIMDMatchesScalarFloat performs a vectorized element-wise arithmetic operation with and without a SIMD kernel.
// Returns an error if the results differ.
func vectorizedSIMDMatchesScalarFloat[T float32 | float64](aSlice, bSlice []T, simdKernel *func(dst, a, b []T), vectorizedOp func(dst, a, b []T), tol *tolerance.Tolerance[T]) error {
	dstScalar := make([]T, len(aSlice))
	dstSIMD := make([]T, len(aSlice))

	saved := *simdKernel
	*simdKernel = nil
	vectorizedOp(dstScalar, aSlice, bSlice)
	*simdKernel = saved

	vectorizedOp(dstSIMD, aSlice, bSlice)

	return tolerance.AssertAllApproxEqual(dstScalar, dstSIMD, tol)
}

// vectorizedSIMDMatchesScalarInt performs a vectorized element-wise arithmetic operation with and without a SIMD kernel.
// Returns an error if the results differ.
func vectorizedSIMDMatchesScalarInt[T int32 | int64](aSlice, bSlice []T, simdKernel *func(dst, a, b []T), vectorizedOp func(dst, a, b []T)) error {
	dstScalar := make([]T, len(aSlice))
	dstSIMD := make([]T, len(aSlice))

	saved := *simdKernel
	*simdKernel = nil
	vectorizedOp(dstScalar, aSlice, bSlice)
	*simdKernel = saved

	vectorizedOp(dstSIMD, aSlice, bSlice)

	for i := range dstScalar {
		if dstScalar[i] != dstSIMD[i] {
			return fmt.Errorf("element[%d]: SIMD=%d scalar=%d", i, dstSIMD[i], dstScalar[i])
		}
	}
	return nil
}

// float32Unit returns a random float32 in the range [-1.0, 1.0).
func float32Unit(rng *rand.Rand) float32 {
	return rng.Float32()*2 - 1
}

// float32Small returns a random float32 in the range [-1e-19, 1e-19).
func float32Small(rng *rand.Rand) float32 {
	return (rng.Float32()*2 - 1) * 1e-19
}

// float32Large returns a random float32 in the range [-1e15, 1e15).
func float32Large(rng *rand.Rand) float32 {
	return (rng.Float32()*2 - 1) * 1e15
}

// float64Unit returns a random float64 in the range [-1.0, 1.0).
func float64Unit(rng *rand.Rand) float64 {
	return rng.Float64()*2 - 1
}

// float64Small returns a random float64 in the range [-1e-154, 1e-154).
func float64Small(rng *rand.Rand) float64 {
	return (rng.Float64()*2 - 1) * 1e-154
}

// float64Large returns a random float64 in the range [-1e150, 1e150).
func float64Large(rng *rand.Rand) float64 {
	return (rng.Float64()*2 - 1) * 1e150
}

// int32Range300 returns a random int32 in the range [-300, 300).
func int32Range300(rng *rand.Rand) int32 {
	return int32(rng.Intn(600) - 300)
}

// int64Range300 returns a random int64 in the range [-300, 300).
func int64Range300(rng *rand.Rand) int64 {
	return rng.Int63n(600) - 300
}

// TestInplaceFloat32_SIMDMatchesScalar verifies that the SIMD inplace
// kernels produce results matching the scalar fallback within float32 ULP noise.
func TestInplaceFloat32_SIMDMatchesScalar(t *testing.T) {
	cases := []InplaceOpCase[float32]{
		{name: "add", kernel: &simdAddInplaceFloat32, op: addInplaceFloat32, aGenerator: float32Unit, bGenerator: float32Unit},
		{name: "sub", kernel: &simdSubInplaceFloat32, op: subInplaceFloat32, aGenerator: float32Unit, bGenerator: float32Unit},
		{name: "mul", kernel: &simdMulInplaceFloat32, op: mulInplaceFloat32, aGenerator: float32Unit, bGenerator: float32Unit},
		{name: "div", kernel: &simdDivInplaceFloat32, op: divInplaceFloat32, aGenerator: float32Unit, bGenerator: float32Unit},

		{name: "add small values", kernel: &simdAddInplaceFloat32, op: addInplaceFloat32, aGenerator: float32Small, bGenerator: float32Small},
		{name: "sub small values", kernel: &simdSubInplaceFloat32, op: subInplaceFloat32, aGenerator: float32Small, bGenerator: float32Small},
		{name: "mul small values", kernel: &simdMulInplaceFloat32, op: mulInplaceFloat32, aGenerator: float32Small, bGenerator: float32Small},
		{name: "div small values", kernel: &simdDivInplaceFloat32, op: divInplaceFloat32, aGenerator: float32Small, bGenerator: float32Small},

		{name: "add large values", kernel: &simdAddInplaceFloat32, op: addInplaceFloat32, aGenerator: float32Large, bGenerator: float32Large},
		{name: "sub large values", kernel: &simdSubInplaceFloat32, op: subInplaceFloat32, aGenerator: float32Large, bGenerator: float32Large},
		{name: "mul large values", kernel: &simdMulInplaceFloat32, op: mulInplaceFloat32, aGenerator: float32Large, bGenerator: float32Large},
		{name: "div large values", kernel: &simdDivInplaceFloat32, op: divInplaceFloat32, aGenerator: float32Large, bGenerator: float32Large},

		{name: "div approaching max float32", kernel: &simdDivInplaceFloat32, op: divInplaceFloat32, aGenerator: float32Large, bGenerator: float32Small},
		{name: "div approaching min float32", kernel: &simdDivInplaceFloat32, op: divInplaceFloat32, aGenerator: float32Small, bGenerator: float32Large},
	}

	rng := rand.New(rand.NewSource(1))
	tol := tolerance.NewDefaultTolerance[float32]()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if *c.kernel == nil {
				t.Skipf("SIMD implementation not available, skipping float32 inplace %s test", c.name)
			}
			for _, n := range simdTestSliceLengths {
				a := make([]float32, n)
				b := make([]float32, n)
				for i := range a {
					a[i] = c.aGenerator(rng)
					b[i] = c.bGenerator(rng)
				}
				if err := inplaceSIMDMatchesScalarFloat(a, b, c.kernel, c.op, tol); err != nil {
					t.Fatalf("float32 inplace %s(n=%d): %v", c.name, n, err)
				}
			}
		})
	}
}

// TestVectorizedFloat32_SIMDMatchesScalar verifies that the SIMD vectorized
// kernels produce results matching the scalar fallback within float32 ULP noise.
func TestVectorizedFloat32_SIMDMatchesScalar(t *testing.T) {
	cases := []VectorizedOpCase[float32]{
		{name: "add", kernel: &simdAddVectorizedFloat32, op: addVectorizedFloat32, aGenerator: float32Unit, bGenerator: float32Unit},
		{name: "sub", kernel: &simdSubVectorizedFloat32, op: subVectorizedFloat32, aGenerator: float32Unit, bGenerator: float32Unit},
		{name: "mul", kernel: &simdMulVectorizedFloat32, op: mulVectorizedFloat32, aGenerator: float32Unit, bGenerator: float32Unit},
		{name: "div", kernel: &simdDivVectorizedFloat32, op: divVectorizedFloat32, aGenerator: float32Unit, bGenerator: float32Unit},

		{name: "add small values", kernel: &simdAddVectorizedFloat32, op: addVectorizedFloat32, aGenerator: float32Small, bGenerator: float32Small},
		{name: "sub small values", kernel: &simdSubVectorizedFloat32, op: subVectorizedFloat32, aGenerator: float32Small, bGenerator: float32Small},
		{name: "mul small values", kernel: &simdMulVectorizedFloat32, op: mulVectorizedFloat32, aGenerator: float32Small, bGenerator: float32Small},
		{name: "div small values", kernel: &simdDivVectorizedFloat32, op: divVectorizedFloat32, aGenerator: float32Small, bGenerator: float32Small},

		{name: "add large values", kernel: &simdAddVectorizedFloat32, op: addVectorizedFloat32, aGenerator: float32Large, bGenerator: float32Large},
		{name: "sub large values", kernel: &simdSubVectorizedFloat32, op: subVectorizedFloat32, aGenerator: float32Large, bGenerator: float32Large},
		{name: "mul large values", kernel: &simdMulVectorizedFloat32, op: mulVectorizedFloat32, aGenerator: float32Large, bGenerator: float32Large},
		{name: "div large values", kernel: &simdDivVectorizedFloat32, op: divVectorizedFloat32, aGenerator: float32Large, bGenerator: float32Large},

		{name: "div approaching max float32", kernel: &simdDivVectorizedFloat32, op: divVectorizedFloat32, aGenerator: float32Large, bGenerator: float32Small},
		{name: "div approaching min float32", kernel: &simdDivVectorizedFloat32, op: divVectorizedFloat32, aGenerator: float32Small, bGenerator: float32Large},
	}

	rng := rand.New(rand.NewSource(1))
	tol := tolerance.NewDefaultTolerance[float32]()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if *c.kernel == nil {
				t.Skipf("SIMD implementation not available, skipping float32 vectorized %s test", c.name)
			}
			for _, n := range simdTestSliceLengths {
				a := make([]float32, n)
				b := make([]float32, n)
				for i := range a {
					a[i] = c.aGenerator(rng)
					b[i] = c.bGenerator(rng)
				}
				if err := vectorizedSIMDMatchesScalarFloat(a, b, c.kernel, c.op, tol); err != nil {
					t.Fatalf("float32 vectorized %s(n=%d): %v", c.name, n, err)
				}
			}
		})
	}
}

// TestInplaceFloat64_SIMDMatchesScalar verifies that the SIMD inplace
// kernels produce results matching the scalar fallback within float64 ULP noise.
func TestInplaceFloat64_SIMDMatchesScalar(t *testing.T) {
	cases := []InplaceOpCase[float64]{
		{name: "add", kernel: &simdAddInplaceFloat64, op: addInplaceFloat64, aGenerator: float64Unit, bGenerator: float64Unit},
		{name: "sub", kernel: &simdSubInplaceFloat64, op: subInplaceFloat64, aGenerator: float64Unit, bGenerator: float64Unit},
		{name: "mul", kernel: &simdMulInplaceFloat64, op: mulInplaceFloat64, aGenerator: float64Unit, bGenerator: float64Unit},
		{name: "div", kernel: &simdDivInplaceFloat64, op: divInplaceFloat64, aGenerator: float64Unit, bGenerator: float64Unit},

		{name: "add small values", kernel: &simdAddInplaceFloat64, op: addInplaceFloat64, aGenerator: float64Small, bGenerator: float64Small},
		{name: "sub small values", kernel: &simdSubInplaceFloat64, op: subInplaceFloat64, aGenerator: float64Small, bGenerator: float64Small},
		{name: "mul small values", kernel: &simdMulInplaceFloat64, op: mulInplaceFloat64, aGenerator: float64Small, bGenerator: float64Small},
		{name: "div small values", kernel: &simdDivInplaceFloat64, op: divInplaceFloat64, aGenerator: float64Small, bGenerator: float64Small},

		{name: "add large values", kernel: &simdAddInplaceFloat64, op: addInplaceFloat64, aGenerator: float64Large, bGenerator: float64Large},
		{name: "sub large values", kernel: &simdSubInplaceFloat64, op: subInplaceFloat64, aGenerator: float64Large, bGenerator: float64Large},
		{name: "mul large values", kernel: &simdMulInplaceFloat64, op: mulInplaceFloat64, aGenerator: float64Large, bGenerator: float64Large},
		{name: "div large values", kernel: &simdDivInplaceFloat64, op: divInplaceFloat64, aGenerator: float64Large, bGenerator: float64Large},

		{name: "div approaching max float64", kernel: &simdDivInplaceFloat64, op: divInplaceFloat64, aGenerator: float64Large, bGenerator: float64Small},
		{name: "div approaching min float64", kernel: &simdDivInplaceFloat64, op: divInplaceFloat64, aGenerator: float64Small, bGenerator: float64Large},
	}

	tol := tolerance.NewDefaultTolerance[float64]()
	rng := rand.New(rand.NewSource(1))

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if *c.kernel == nil {
				t.Skipf("SIMD implementation not available, skipping float64 inplace %s test", c.name)
			}
			for _, n := range simdTestSliceLengths {
				a := make([]float64, n)
				b := make([]float64, n)
				for i := range a {
					a[i] = c.aGenerator(rng)
					b[i] = c.bGenerator(rng)
				}
				if err := inplaceSIMDMatchesScalarFloat(a, b, c.kernel, c.op, tol); err != nil {
					t.Fatalf("float64 inplace %s(n=%d): %v", c.name, n, err)
				}
			}
		})
	}
}

// TestVectorizedFloat64_SIMDMatchesScalar verifies that the SIMD vectorized
// kernels produce results matching the scalar fallback within float64 ULP noise.
func TestVectorizedFloat64_SIMDMatchesScalar(t *testing.T) {
	cases := []VectorizedOpCase[float64]{
		{name: "add", kernel: &simdAddVectorizedFloat64, op: addVectorizedFloat64, aGenerator: float64Unit, bGenerator: float64Unit},
		{name: "sub", kernel: &simdSubVectorizedFloat64, op: subVectorizedFloat64, aGenerator: float64Unit, bGenerator: float64Unit},
		{name: "mul", kernel: &simdMulVectorizedFloat64, op: mulVectorizedFloat64, aGenerator: float64Unit, bGenerator: float64Unit},
		{name: "div", kernel: &simdDivVectorizedFloat64, op: divVectorizedFloat64, aGenerator: float64Unit, bGenerator: float64Unit},

		{name: "add small values", kernel: &simdAddVectorizedFloat64, op: addVectorizedFloat64, aGenerator: float64Small, bGenerator: float64Small},
		{name: "sub small values", kernel: &simdSubVectorizedFloat64, op: subVectorizedFloat64, aGenerator: float64Small, bGenerator: float64Small},
		{name: "mul small values", kernel: &simdMulVectorizedFloat64, op: mulVectorizedFloat64, aGenerator: float64Small, bGenerator: float64Small},
		{name: "div small values", kernel: &simdDivVectorizedFloat64, op: divVectorizedFloat64, aGenerator: float64Small, bGenerator: float64Small},

		{name: "add large values", kernel: &simdAddVectorizedFloat64, op: addVectorizedFloat64, aGenerator: float64Large, bGenerator: float64Large},
		{name: "sub large values", kernel: &simdSubVectorizedFloat64, op: subVectorizedFloat64, aGenerator: float64Large, bGenerator: float64Large},
		{name: "mul large values", kernel: &simdMulVectorizedFloat64, op: mulVectorizedFloat64, aGenerator: float64Large, bGenerator: float64Large},
		{name: "div large values", kernel: &simdDivVectorizedFloat64, op: divVectorizedFloat64, aGenerator: float64Large, bGenerator: float64Large},

		{name: "div approaching max float64", kernel: &simdDivVectorizedFloat64, op: divVectorizedFloat64, aGenerator: float64Large, bGenerator: float64Small},
		{name: "div approaching min float64", kernel: &simdDivVectorizedFloat64, op: divVectorizedFloat64, aGenerator: float64Small, bGenerator: float64Large},
	}

	tol := tolerance.NewDefaultTolerance[float64]()
	rng := rand.New(rand.NewSource(1))

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if *c.kernel == nil {
				t.Skipf("SIMD implementation not available, skipping float64 vectorized %s test", c.name)
			}
			for _, n := range simdTestSliceLengths {
				a := make([]float64, n)
				b := make([]float64, n)
				for i := range a {
					a[i] = c.aGenerator(rng)
					b[i] = c.bGenerator(rng)
				}
				if err := vectorizedSIMDMatchesScalarFloat(a, b, c.kernel, c.op, tol); err != nil {
					t.Fatalf("float64 vectorized %s(n=%d): %v", c.name, n, err)
				}
			}
		})
	}
}

// TestInplaceInt32_SIMDMatchesScalar verifies that the SIMD inplace
// kernels produce results matching the scalar fallback.
func TestInplaceInt32_SIMDMatchesScalar(t *testing.T) {
	cases := []InplaceOpCase[int32]{
		{name: "add", kernel: &simdAddInplaceInt32, op: addInplaceInt32, aGenerator: int32Range300, bGenerator: int32Range300},
		{name: "sub", kernel: &simdSubInplaceInt32, op: subInplaceInt32, aGenerator: int32Range300, bGenerator: int32Range300},
		{name: "mul", kernel: &simdMulInplaceInt32, op: mulInplaceInt32, aGenerator: int32Range300, bGenerator: int32Range300},
	}

	rng := rand.New(rand.NewSource(1))

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if *c.kernel == nil {
				t.Skipf("SIMD implementation not available, skipping int32 inplace %s test", c.name)
			}
			for _, n := range simdTestSliceLengths {
				a := make([]int32, n)
				b := make([]int32, n)
				for i := range a {
					a[i] = c.aGenerator(rng)
					b[i] = c.bGenerator(rng)
				}
				if err := inplaceSIMDMatchesScalarInt(a, b, c.kernel, c.op); err != nil {
					t.Fatalf("int32 inplace %s(n=%d): %v", c.name, n, err)
				}
			}
		})
	}
}

// TestVectorizedInt32_SIMDMatchesScalar verifies that the SIMD vectorized
// kernels produce results matching the scalar fallback.
func TestVectorizedInt32_SIMDMatchesScalar(t *testing.T) {
	cases := []VectorizedOpCase[int32]{
		{name: "add", kernel: &simdAddVectorizedInt32, op: addVectorizedInt32, aGenerator: int32Range300, bGenerator: int32Range300},
		{name: "sub", kernel: &simdSubVectorizedInt32, op: subVectorizedInt32, aGenerator: int32Range300, bGenerator: int32Range300},
		{name: "mul", kernel: &simdMulVectorizedInt32, op: mulVectorizedInt32, aGenerator: int32Range300, bGenerator: int32Range300},
	}

	rng := rand.New(rand.NewSource(1))

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if *c.kernel == nil {
				t.Skipf("SIMD implementation not available, skipping int32 vectorized %s test", c.name)
			}
			for _, n := range simdTestSliceLengths {
				a := make([]int32, n)
				b := make([]int32, n)
				for i := range a {
					a[i] = c.aGenerator(rng)
					b[i] = c.bGenerator(rng)
				}
				if err := vectorizedSIMDMatchesScalarInt(a, b, c.kernel, c.op); err != nil {
					t.Fatalf("int32 vectorized %s(n=%d): %v", c.name, n, err)
				}
			}
		})
	}
}

// TestInplaceInt64_SIMDMatchesScalar verifies that the SIMD inplace
// kernels produce results matching the scalar fallback.
func TestInplaceInt64_SIMDMatchesScalar(t *testing.T) {
	cases := []InplaceOpCase[int64]{
		{name: "add", kernel: &simdAddInplaceInt64, op: addInplaceInt64, aGenerator: int64Range300, bGenerator: int64Range300},
		{name: "sub", kernel: &simdSubInplaceInt64, op: subInplaceInt64, aGenerator: int64Range300, bGenerator: int64Range300},
		{name: "mul", kernel: &simdMulInplaceInt64, op: mulInplaceInt64, aGenerator: int64Range300, bGenerator: int64Range300},
	}

	rng := rand.New(rand.NewSource(1))

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if *c.kernel == nil {
				t.Skipf("SIMD implementation not available, skipping int64 inplace %s test", c.name)
			}
			for _, n := range simdTestSliceLengths {
				a := make([]int64, n)
				b := make([]int64, n)
				for i := range a {
					a[i] = c.aGenerator(rng)
					b[i] = c.bGenerator(rng)
				}
				if err := inplaceSIMDMatchesScalarInt(a, b, c.kernel, c.op); err != nil {
					t.Fatalf("int64 inplace %s(n=%d): %v", c.name, n, err)
				}
			}
		})
	}
}

// TestVectorizedInt64_SIMDMatchesScalar verifies that the SIMD vectorized
// kernels produce results matching the scalar fallback.
func TestVectorizedInt64_SIMDMatchesScalar(t *testing.T) {
	cases := []VectorizedOpCase[int64]{
		{name: "add", kernel: &simdAddVectorizedInt64, op: addVectorizedInt64, aGenerator: int64Range300, bGenerator: int64Range300},
		{name: "sub", kernel: &simdSubVectorizedInt64, op: subVectorizedInt64, aGenerator: int64Range300, bGenerator: int64Range300},
		{name: "mul", kernel: &simdMulVectorizedInt64, op: mulVectorizedInt64, aGenerator: int64Range300, bGenerator: int64Range300},
	}

	rng := rand.New(rand.NewSource(1))

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if *c.kernel == nil {
				t.Skipf("SIMD implementation not available, skipping int64 vectorized %s test", c.name)
			}
			for _, n := range simdTestSliceLengths {
				a := make([]int64, n)
				b := make([]int64, n)
				for i := range a {
					a[i] = c.aGenerator(rng)
					b[i] = c.bGenerator(rng)
				}
				if err := vectorizedSIMDMatchesScalarInt(a, b, c.kernel, c.op); err != nil {
					t.Fatalf("int64 vectorized %s(n=%d): %v", c.name, n, err)
				}
			}
		})
	}
}
