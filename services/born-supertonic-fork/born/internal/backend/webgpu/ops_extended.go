//go:build windows || linux

package webgpu

import (
	"github.com/born-ml/born/internal/tensor"
)

// Extended operations - GPU implementations using WGSL shaders.

// Scalar operations - use runScalarOp for GPU execution.

// MulScalar multiplies tensor elements by a scalar on GPU.
func (b *Backend) MulScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	s := toFloat32(scalar)
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runScalarOpLazy(x, s, "scalarMul", scalarMulShader)
	} else {
		result, err = b.runScalarOp(x, s, "scalarMul", scalarMulShader)
	}
	if err != nil {
		panic("webgpu: MulScalar: " + err.Error())
	}
	return result
}

// AddScalar adds a scalar to tensor elements on GPU.
func (b *Backend) AddScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	s := toFloat32(scalar)
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runScalarOpLazy(x, s, "scalarAdd", scalarAddShader)
	} else {
		result, err = b.runScalarOp(x, s, "scalarAdd", scalarAddShader)
	}
	if err != nil {
		panic("webgpu: AddScalar: " + err.Error())
	}
	return result
}

// SubScalar subtracts a scalar from tensor elements on GPU.
func (b *Backend) SubScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	s := toFloat32(scalar)
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runScalarOpLazy(x, -s, "scalarAdd", scalarAddShader) // x - s = x + (-s)
	} else {
		result, err = b.runScalarOp(x, -s, "scalarAdd", scalarAddShader)
	}
	if err != nil {
		panic("webgpu: SubScalar: " + err.Error())
	}
	return result
}

// DivScalar divides tensor elements by a scalar on GPU.
func (b *Backend) DivScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	s := toFloat32(scalar)
	if s == 0 {
		panic("webgpu: DivScalar: division by zero")
	}
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runScalarOpLazy(x, 1.0/s, "scalarMul", scalarMulShader) // x / s = x * (1/s)
	} else {
		result, err = b.runScalarOp(x, 1.0/s, "scalarMul", scalarMulShader)
	}
	if err != nil {
		panic("webgpu: DivScalar: " + err.Error())
	}
	return result
}

// toFloat32 converts any numeric type to float32.
func toFloat32(v any) float32 {
	switch val := v.(type) {
	case float32:
		return val
	case float64:
		return float32(val)
	case int:
		return float32(val)
	case int32:
		return float32(val)
	case int64:
		return float32(val)
	default:
		panic("webgpu: unsupported scalar type")
	}
}

// Math operations.

// Log computes natural logarithm element-wise on GPU.
func (b *Backend) Log(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "log", logShader)
	} else {
		result, err = b.runUnaryOp(x, "log", logShader)
	}
	if err != nil {
		panic("webgpu: Log: " + err.Error())
	}
	return result
}

// Activation functions.

// Softmax applies softmax along the specified dimension.
// Supports N-dimensional tensors with dim=-1 (last dimension).
func (b *Backend) Softmax(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dim
	if dim < 0 {
		dim = ndim + dim
	}

	// Only support softmax on last dimension
	if dim != ndim-1 {
		panic("webgpu: Softmax currently only supports dim=-1 (last dimension)")
	}

	// For 2D tensors, use GPU softmax directly
	if ndim == 2 {
		var result *tensor.RawTensor
		var err error
		if b.LazyMode {
			result, err = b.runSoftmaxLazy(x)
		} else {
			result, err = b.runSoftmax(x)
		}
		if err != nil {
			panic("webgpu: Softmax: " + err.Error())
		}
		return result
	}

	// For N-D tensors: flatten → 2D softmax → reshape back
	// [d0, d1, ..., d_{n-2}, d_{n-1}] → [d0*d1*...*d_{n-2}, d_{n-1}]
	lastDim := shape[ndim-1]
	batchSize := 1
	for i := 0; i < ndim-1; i++ {
		batchSize *= shape[i]
	}

	// Reshape to 2D
	flat := b.Reshape(x, tensor.Shape{batchSize, lastDim})

	// Apply 2D softmax
	var result2D *tensor.RawTensor
	var err error
	if b.LazyMode {
		result2D, err = b.runSoftmaxLazy(flat)
	} else {
		result2D, err = b.runSoftmax(flat)
	}
	if err != nil {
		panic("webgpu: Softmax: " + err.Error())
	}

	// Reshape back to original shape
	return b.Reshape(result2D, shape)
}

// Comparison operations.

// Greater performs element-wise greater-than comparison on GPU.
// Always returns float32 tensor (0.0 for false, 1.0 for true).
func (b *Backend) Greater(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.comparisonOp(a, other, "greater", greaterShader)
}

// Lower performs element-wise less-than comparison on GPU.
// Always returns float32 tensor (0.0 for false, 1.0 for true).
func (b *Backend) Lower(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.comparisonOp(a, other, "lower", lowerShader)
}

// GreaterEqual performs element-wise greater-or-equal comparison on GPU.
// Always returns float32 tensor (0.0 for false, 1.0 for true).
func (b *Backend) GreaterEqual(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.comparisonOp(a, other, "greaterEqual", greaterEqualShader)
}

// LowerEqual performs element-wise less-or-equal comparison on GPU.
// Always returns float32 tensor (0.0 for false, 1.0 for true).
func (b *Backend) LowerEqual(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.comparisonOp(a, other, "lowerEqual", lowerEqualShader)
}

// Equal performs element-wise equality comparison on GPU.
// Always returns float32 tensor (0.0 for false, 1.0 for true).
func (b *Backend) Equal(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.comparisonOp(a, other, "equal", equalShader)
}

// NotEqual performs element-wise inequality comparison on GPU.
// Always returns float32 tensor (0.0 for false, 1.0 for true).
func (b *Backend) NotEqual(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.comparisonOp(a, other, "notEqual", notEqualShader)
}

// comparisonOp dispatches a comparison op with LazyMode support.
// Comparison shaders have the same bind group layout as binary ops (2 in, 1 out, params).
func (b *Backend) comparisonOp(a, other *tensor.RawTensor, shaderName, shaderCode string) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode && a.DType() == tensor.Float32 {
		result, err = b.runBinaryOpLazy(a, other, shaderName, shaderCode)
	} else {
		result, err = b.runComparisonOp(a, other, shaderName, shaderCode)
	}
	if err != nil {
		panic("webgpu: " + shaderName + ": " + err.Error())
	}
	return result
}

// Boolean operations.

// Or performs element-wise logical OR on GPU.
// Supports mixed dtypes by casting to float32 (for boolean tensors from different sources).
func (b *Backend) Or(a, other *tensor.RawTensor) *tensor.RawTensor {
	// Cast to float32 if dtypes differ (common for boolean results from different tensor types)
	aFloat := a
	otherFloat := other
	if a.DType() != tensor.Float32 {
		aFloat = b.Cast(a, tensor.Float32)
	}
	if other.DType() != tensor.Float32 {
		otherFloat = b.Cast(other, tensor.Float32)
	}

	if b.LazyMode && aFloat.DType() == tensor.Float32 {
		result, err := b.runBinaryOpLazy(aFloat, otherFloat, "or", orShader)
		if err != nil {
			panic("webgpu: Or: " + err.Error())
		}
		return result
	}
	result, err := b.runBinaryOp(aFloat, otherFloat, "or", orShader)
	if err != nil {
		panic("webgpu: Or: " + err.Error())
	}
	return result
}

// And performs element-wise logical AND on GPU.
// Supports mixed dtypes by casting to float32 (for boolean tensors from different sources).
func (b *Backend) And(a, other *tensor.RawTensor) *tensor.RawTensor {
	// Cast to float32 if dtypes differ (common for boolean results from different tensor types)
	aFloat := a
	otherFloat := other
	if a.DType() != tensor.Float32 {
		aFloat = b.Cast(a, tensor.Float32)
	}
	if other.DType() != tensor.Float32 {
		otherFloat = b.Cast(other, tensor.Float32)
	}

	if b.LazyMode && aFloat.DType() == tensor.Float32 {
		result, err := b.runBinaryOpLazy(aFloat, otherFloat, "and", andShader)
		if err != nil {
			panic("webgpu: And: " + err.Error())
		}
		return result
	}
	result, err := b.runBinaryOp(aFloat, otherFloat, "and", andShader)
	if err != nil {
		panic("webgpu: And: " + err.Error())
	}
	return result
}

// Not performs element-wise logical NOT on GPU.
func (b *Backend) Not(x *tensor.RawTensor) *tensor.RawTensor {
	if b.LazyMode && x.DType() == tensor.Float32 {
		result, err := b.runUnaryOpLazy(x, "not", notShader)
		if err != nil {
			panic("webgpu: Not: " + err.Error())
		}
		return result
	}
	result, err := b.runUnaryOp(x, "not", notShader)
	if err != nil {
		panic("webgpu: Not: " + err.Error())
	}
	return result
}

// Reduction operations.

// Sum computes the sum of all elements on GPU.
func (b *Backend) Sum(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runSumLazy(x)
	} else {
		result, err = b.runSum(x)
	}
	if err != nil {
		panic("webgpu: Sum: " + err.Error())
	}
	return result
}

// Argmax returns indices of maximum values along dimension on GPU.
func (b *Backend) Argmax(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	result, err := b.runArgmax(x, dim)
	if err != nil {
		panic("webgpu: Argmax: " + err.Error())
	}
	return result
}

// Shape operations.

// Expand broadcasts tensor to new shape.
// GPU-accelerated for up to 6D tensors.
func (b *Backend) Expand(x *tensor.RawTensor, newShape tensor.Shape) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runExpandLazy(x, newShape)
	} else {
		result, err = b.runExpand(x, newShape)
	}
	if err != nil {
		panic("webgpu: Expand: " + err.Error())
	}
	return result
}

// Type conversion.

// Cast converts tensor to different data type.
// Supports float32 and int32 as target types.
func (b *Backend) Cast(x *tensor.RawTensor, dtype tensor.DataType) *tensor.RawTensor {
	if x.DType() == dtype {
		// Same dtype: return input directly in LazyMode to avoid GPU→CPU→GPU round-trip.
		// In non-lazy mode, copy to a new CPU-backed tensor (original behavior).
		if b.LazyMode {
			return x
		}
		result, err := tensor.NewRaw(x.Shape(), dtype, tensor.WebGPU)
		if err != nil {
			panic("webgpu: Cast: " + err.Error())
		}
		copy(result.Data(), x.Data())
		return result
	}

	result, err := tensor.NewRaw(x.Shape(), dtype, tensor.WebGPU)
	if err != nil {
		panic("webgpu: Cast: " + err.Error())
	}

	// Route by target dtype
	switch dtype {
	case tensor.Float32:
		b.castToFloat32(x, result)
	case tensor.Int32:
		b.castToInt32(x, result)
	default:
		panic("webgpu: Cast: unsupported target type " + dtype.String())
	}

	return result
}

// castToFloat32 converts any supported dtype to float32.
func (b *Backend) castToFloat32(x, result *tensor.RawTensor) {
	dst := result.AsFloat32()
	switch x.DType() {
	case tensor.Float64:
		src := x.AsFloat64()
		for i, v := range src {
			dst[i] = float32(v)
		}
	case tensor.Int32:
		src := x.AsInt32()
		for i, v := range src {
			dst[i] = float32(v)
		}
	case tensor.Int64:
		src := x.AsInt64()
		for i, v := range src {
			dst[i] = float32(v)
		}
	default:
		panic("webgpu: Cast: unsupported source type for float32: " + x.DType().String())
	}
}

// castToInt32 converts any supported dtype to int32.
func (b *Backend) castToInt32(x, result *tensor.RawTensor) {
	dst := result.AsInt32()
	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		for i, v := range src {
			dst[i] = int32(v)
		}
	case tensor.Float64:
		src := x.AsFloat64()
		for i, v := range src {
			dst[i] = int32(v)
		}
	case tensor.Int64:
		src := x.AsInt64()
		for i, v := range src {
			dst[i] = int32(v) //nolint:gosec // G115: safe, int64→int32 truncation accepted for GPU cast op
		}
	default:
		panic("webgpu: Cast: unsupported source type for int32: " + x.DType().String())
	}
}
