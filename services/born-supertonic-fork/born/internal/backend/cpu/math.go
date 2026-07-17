package cpu

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// Log computes element-wise natural logarithm: ln(x).
func (cpu *CPUBackend) Log(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("log: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		for i, v := range src {
			if v <= 0 {
				panic(fmt.Sprintf("log: non-positive value at index %d: %f", i, v))
			}
			dst[i] = float32(math.Log(float64(v)))
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		for i, v := range src {
			if v <= 0 {
				panic(fmt.Sprintf("log: non-positive value at index %d: %f", i, v))
			}
			dst[i] = math.Log(v)
		}
	default:
		panic(fmt.Sprintf("log: unsupported dtype %s (only float32/float64 supported)", x.DType()))
	}

	return result
}

// Sqrt computes element-wise square root: sqrt(x).
func (cpu *CPUBackend) Sqrt(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("sqrt: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		for i, v := range src {
			if v < 0 {
				panic(fmt.Sprintf("sqrt: negative value at index %d: %f", i, v))
			}
			dst[i] = float32(math.Sqrt(float64(v)))
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		for i, v := range src {
			if v < 0 {
				panic(fmt.Sprintf("sqrt: negative value at index %d: %f", i, v))
			}
			dst[i] = math.Sqrt(v)
		}
	default:
		panic(fmt.Sprintf("sqrt: unsupported dtype %s (only float32/float64 supported)", x.DType()))
	}

	return result
}

// Rsqrt computes element-wise reciprocal square root: 1/sqrt(x).
// This is optimized for use in normalization layers (RMSNorm, LayerNorm).
func (cpu *CPUBackend) Rsqrt(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("rsqrt: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		for i, v := range src {
			if v <= 0 {
				panic(fmt.Sprintf("rsqrt: non-positive value at index %d: %f", i, v))
			}
			dst[i] = 1.0 / float32(math.Sqrt(float64(v)))
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		for i, v := range src {
			if v <= 0 {
				panic(fmt.Sprintf("rsqrt: non-positive value at index %d: %f", i, v))
			}
			dst[i] = 1.0 / math.Sqrt(v)
		}
	default:
		panic(fmt.Sprintf("rsqrt: unsupported dtype %s (only float32/float64 supported)", x.DType()))
	}

	return result
}

// Cos computes element-wise cosine: cos(x).
func (cpu *CPUBackend) Cos(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("cos: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		for i, v := range src {
			dst[i] = float32(math.Cos(float64(v)))
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		for i, v := range src {
			dst[i] = math.Cos(v)
		}
	default:
		panic(fmt.Sprintf("cos: unsupported dtype %s (only float32/float64 supported)", x.DType()))
	}

	return result
}

// Sin computes element-wise sine: sin(x).
func (cpu *CPUBackend) Sin(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("sin: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		for i, v := range src {
			dst[i] = float32(math.Sin(float64(v)))
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		for i, v := range src {
			dst[i] = math.Sin(v)
		}
	default:
		panic(fmt.Sprintf("sin: unsupported dtype %s (only float32/float64 supported)", x.DType()))
	}

	return result
}

// Erf computes element-wise error function: erf(x).
func (cpu *CPUBackend) Erf(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("erf: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		for i, v := range src {
			dst[i] = float32(math.Erf(float64(v)))
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		for i, v := range src {
			dst[i] = math.Erf(v)
		}
	default:
		panic(fmt.Sprintf("erf: unsupported dtype %s (only float32/float64 supported)", x.DType()))
	}

	return result
}

// signUint8 computes sign for unsigned bytes: 0 → 0, positive → 1.
func signUint8(dst, src []uint8) {
	for i := range src {
		if src[i] > 0 {
			dst[i] = 1
		} else {
			dst[i] = 0
		}
	}
}

// signInts computes sign for signed integers (int32, int64).
func signInts[T int32 | int64](dst, src []T) {
	for i, v := range src {
		switch {
		case v > 0:
			dst[i] = 1
		case v < 0:
			dst[i] = -1
		default:
			dst[i] = 0
		}
	}
}

// signFloats computes sign for floating-point numbers (float32, float64) with NaN preservation.
func signFloats[T float32 | float64](dst, src []T) {
	for i, v := range src {
		switch {
		case math.IsNaN(float64(v)):
			dst[i] = T(math.NaN())
		case v > 0:
			dst[i] = T(1.0)
		case v < 0:
			dst[i] = T(-1.0)
		default:
			dst[i] = T(0.0)
		}
	}
}

// Sign computes element-wise sign function: sign(x).
func (cpu *CPUBackend) Sign(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("sign: %v", err))
	}

	switch x.DType() {
	case tensor.Uint8:
		dst := result.AsUint8()
		src := x.AsUint8()
		if simdSignUint8 != nil {
			simdSignUint8(dst, src)
		} else {
			signUint8(dst, src)
		}
	case tensor.Int32:
		dst := result.AsInt32()
		src := x.AsInt32()
		if simdSignInt32 != nil {
			simdSignInt32(dst, src)
		} else {
			signInts(dst, src)
		}
	case tensor.Int64:
		dst := result.AsInt64()
		src := x.AsInt64()
		if simdSignInt64 != nil {
			simdSignInt64(dst, src)
		} else {
			signInts(dst, src)
		}
	case tensor.Float32:
		dst := result.AsFloat32()
		src := x.AsFloat32()
		if simdSignFloat32 != nil {
			simdSignFloat32(dst, src)
		} else {
			signFloats(dst, src)
		}
	case tensor.Float64:
		dst := result.AsFloat64()
		src := x.AsFloat64()
		if simdSignFloat64 != nil {
			simdSignFloat64(dst, src)
		} else {
			signFloats(dst, src)
		}
	default:
		panic(fmt.Sprintf("sign: unsupported dtype %s (only uint8/int32/int64/float32/float64 supported)", x.DType()))
	}

	return result
}

// absUint8 — identity (absolute value of unsigned is itself).
func absUint8(src, dst []uint8) {
	copy(dst, src)
}

// absInts — two's-complement wrapping abs for signed integers.
// Note: abs(MinInt) == MinInt (wraparound), matching Burn and NumPy/PyTorch.
func absInts[T int32 | int64](src, dst []T) {
	for i, v := range src {
		if v < 0 {
			dst[i] = -v // wraps for MinInt, which is the intended semantics
		} else {
			dst[i] = v
		}
	}
}

// absFloats — float abs via math.Abs (handles NaN/±Inf correctly).
func absFloats[T float32 | float64](src, dst []T) {
	for i, v := range src {
		dst[i] = T(math.Abs(float64(v)))
	}
}

// Abs computes element-wise absolute value: abs(x).
func (cpu *CPUBackend) Abs(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("abs: %v", err))
	}

	switch x.DType() {
	case tensor.Uint8:
		absUint8(x.AsUint8(), result.AsUint8())
	case tensor.Int32:
		absInts(x.AsInt32(), result.AsInt32())
	case tensor.Int64:
		absInts(x.AsInt64(), result.AsInt64())
	case tensor.Float32:
		absFloats(x.AsFloat32(), result.AsFloat32())
	case tensor.Float64:
		absFloats(x.AsFloat64(), result.AsFloat64())
	default:
		panic(fmt.Sprintf("abs: unsupported dtype %s (only int32/int64/float32/float64 supported)", x.DType()))
	}

	return result
}
