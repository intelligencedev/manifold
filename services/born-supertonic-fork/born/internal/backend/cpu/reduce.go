package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// SumDim sums tensor elements along the specified dimension.
//
// Parameters:
//   - dim: dimension to reduce (supports negative indexing: -1 = last dim)
//   - keepDim: if true, keep the reduced dimension with size 1; if false, remove it
//
// Example:
//
//	x := tensor.Randn[float32]([]int{2, 3, 4}, backend)
//	y := backend.SumDim(x, -1, true)   // shape: [2, 3, 1]
//	z := backend.SumDim(x, -1, false)  // shape: [2, 3]
func (cpu *CPUBackend) SumDim(x *tensor.RawTensor, dim int, keepDim bool) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// Validate dimension
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("sumdim: dimension %d out of range for %dD tensor", dim, ndim))
	}

	// Calculate output shape
	var outShape tensor.Shape
	if keepDim {
		outShape = shape.Clone()
		outShape[dim] = 1
	} else {
		outShape = make(tensor.Shape, 0, ndim-1)
		for i := 0; i < ndim; i++ {
			if i != dim {
				outShape = append(outShape, shape[i])
			}
		}
	}

	// Create result tensor
	result, err := tensor.NewRaw(outShape, x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("sumdim: %v", err))
	}

	// Perform reduction
	switch x.DType() {
	case tensor.Float32:
		sumDimFloat32(x.AsFloat32(), result.AsFloat32(), shape, dim)
	case tensor.Float64:
		sumDimFloat64(x.AsFloat64(), result.AsFloat64(), shape, dim)
	default:
		panic(fmt.Sprintf("sumdim: unsupported dtype %s (only float32/float64 supported)", x.DType()))
	}

	return result
}

// MeanDim computes the mean of tensor elements along the specified dimension.
//
// Parameters:
//   - dim: dimension to reduce (supports negative indexing: -1 = last dim)
//   - keepDim: if true, keep the reduced dimension with size 1; if false, remove it
//
// Example:
//
//	x := tensor.Randn[float32]([]int{2, 3, 4}, backend)
//	y := backend.MeanDim(x, -1, true)   // shape: [2, 3, 1]
//	z := backend.MeanDim(x, -1, false)  // shape: [2, 3]
func (cpu *CPUBackend) MeanDim(x *tensor.RawTensor, dim int, keepDim bool) *tensor.RawTensor {
	// Sum along dimension
	sumResult := cpu.SumDim(x, dim, keepDim)

	// Normalize negative dimension for division
	shape := x.Shape()
	ndim := len(shape)
	if dim < 0 {
		dim = ndim + dim
	}

	// Divide by the size of the reduced dimension
	divisor := float64(shape[dim])

	// Divide element-wise
	switch sumResult.DType() {
	case tensor.Float32:
		data := sumResult.AsFloat32()
		divisorF32 := float32(divisor)
		for i := range data {
			data[i] /= divisorF32
		}
	case tensor.Float64:
		data := sumResult.AsFloat64()
		for i := range data {
			data[i] /= divisor
		}
	default:
		panic(fmt.Sprintf("meandim: unsupported dtype %s (only float32/float64 supported)", sumResult.DType()))
	}

	return sumResult
}

// sumDimFloat32 performs dimension reduction for float32 tensors.
func sumDimFloat32(data, result []float32, shape tensor.Shape, dim int) {
	// Initialize result to zero
	for i := range result {
		result[i] = 0
	}

	// Calculate strides for input
	strides := shape.ComputeStrides()
	numElements := shape.NumElements()

	// Calculate output strides (with reduced dimension size = 1)
	outShape := shape.Clone()
	outShape[dim] = 1
	outStrides := outShape.ComputeStrides()

	// Iterate over all input elements
	for i := 0; i < numElements; i++ {
		// Compute multi-dimensional index
		outIdx := 0
		temp := i
		for d := 0; d < len(shape); d++ {
			coord := temp / strides[d]
			temp %= strides[d]

			// For the reduced dimension, we always use coordinate 0 in output
			if d != dim {
				outIdx += coord * outStrides[d]
			}
		}

		result[outIdx] += data[i]
	}
}

// sumDimFloat64 performs dimension reduction for float64 tensors.
func sumDimFloat64(data, result []float64, shape tensor.Shape, dim int) {
	// Initialize result to zero
	for i := range result {
		result[i] = 0
	}

	// Calculate strides for input
	strides := shape.ComputeStrides()
	numElements := shape.NumElements()

	// Calculate output strides (with reduced dimension size = 1)
	outShape := shape.Clone()
	outShape[dim] = 1
	outStrides := outShape.ComputeStrides()

	// Iterate over all input elements
	for i := 0; i < numElements; i++ {
		// Compute multi-dimensional index
		outIdx := 0
		temp := i
		for d := 0; d < len(shape); d++ {
			coord := temp / strides[d]
			temp %= strides[d]

			// For the reduced dimension, we always use coordinate 0 in output
			if d != dim {
				outIdx += coord * outStrides[d]
			}
		}

		result[outIdx] += data[i]
	}
}

// Sum computes the total sum of all elements in the tensor (scalar result).
func (cpu *CPUBackend) Sum(x *tensor.RawTensor) *tensor.RawTensor {
	// Result is a scalar (empty shape)
	result, err := tensor.NewRaw(tensor.Shape{}, x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("sum: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		if simdSumFloat32 != nil {
			simdSumFloat32(dst, src)
		} else {
			sumScalar(dst, src)
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		if simdSumFloat64 != nil {
			simdSumFloat64(dst, src)
		} else {
			sumScalar(dst, src)
		}
	case tensor.Int32:
		src := x.AsInt32()
		dst := result.AsInt32()
		if simdSumInt32 != nil {
			simdSumInt32(dst, src)
		} else {
			sumScalar(dst, src)
		}
	case tensor.Int64:
		src := x.AsInt64()
		dst := result.AsInt64()
		if simdSumInt64 != nil {
			simdSumInt64(dst, src)
		} else {
			sumScalar(dst, src)
		}
	default:
		panic(fmt.Sprintf("sum: unsupported dtype %s", x.DType()))
	}

	return result
}

// sumScalar computes dst[0] = sum(src[i]) using a naive scalar loop.
func sumScalar[T float32 | float64 | int32 | int64](dst, src []T) {
	sum := T(0)
	for _, v := range src {
		sum += v
	}
	dst[0] = sum
}

// Argmax returns the index of the maximum value along the specified dimension.
func (cpu *CPUBackend) Argmax(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// Validate dimension
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("argmax: dimension %d out of range for %dD tensor", dim, ndim))
	}

	// Calculate output shape (remove the reduced dimension)
	outShape := make(tensor.Shape, 0, ndim-1)
	for i := 0; i < ndim; i++ {
		if i != dim {
			outShape = append(outShape, shape[i])
		}
	}

	// Create result tensor (int32)
	result, err := tensor.NewRaw(outShape, tensor.Int32, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("argmax: %v", err))
	}

	// Perform reduction
	switch x.DType() {
	case tensor.Float32:
		argmaxFloat32(x.AsFloat32(), result.AsInt32(), shape, dim)
	case tensor.Float64:
		argmaxFloat64(x.AsFloat64(), result.AsInt32(), shape, dim)
	case tensor.Int32:
		argmaxInt32(x.AsInt32(), result.AsInt32(), shape, dim)
	case tensor.Int64:
		argmaxInt64(x.AsInt64(), result.AsInt32(), shape, dim)
	default:
		panic(fmt.Sprintf("argmax: unsupported dtype %s", x.DType()))
	}

	return result
}

func argmaxFloat32(data []float32, result []int32, shape tensor.Shape, dim int) {
	strides := shape.ComputeStrides()
	dimSize := shape[dim]
	dimStride := strides[dim]

	// Number of reduction groups
	numGroups := 1
	for i := range shape {
		if i != dim {
			numGroups *= shape[i]
		}
	}

	// For each group
	resultIdx := 0
	for group := 0; group < numGroups; group++ {
		// Compute base index for this group
		baseIdx := 0
		remaining := group
		for i := 0; i < len(shape); i++ {
			if i == dim {
				continue
			}
			coord := remaining % shape[i]
			remaining /= shape[i]
			baseIdx += coord * strides[i]
		}

		// Find argmax along dim
		maxVal := data[baseIdx]
		maxIdx := int32(0)
		for i := 1; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			if data[idx] > maxVal {
				maxVal = data[idx]
				maxIdx = int32(i)
			}
		}

		result[resultIdx] = maxIdx
		resultIdx++
	}
}

func argmaxFloat64(data []float64, result []int32, shape tensor.Shape, dim int) {
	strides := shape.ComputeStrides()
	dimSize := shape[dim]
	dimStride := strides[dim]

	numGroups := 1
	for i := range shape {
		if i != dim {
			numGroups *= shape[i]
		}
	}

	resultIdx := 0
	for group := 0; group < numGroups; group++ {
		baseIdx := 0
		remaining := group
		for i := 0; i < len(shape); i++ {
			if i == dim {
				continue
			}
			coord := remaining % shape[i]
			remaining /= shape[i]
			baseIdx += coord * strides[i]
		}

		maxVal := data[baseIdx]
		maxIdx := int32(0)
		for i := 1; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			if data[idx] > maxVal {
				maxVal = data[idx]
				maxIdx = int32(i)
			}
		}

		result[resultIdx] = maxIdx
		resultIdx++
	}
}

func argmaxInt32(data, result []int32, shape tensor.Shape, dim int) {
	strides := shape.ComputeStrides()
	dimSize := shape[dim]
	dimStride := strides[dim]

	numGroups := 1
	for i := range shape {
		if i != dim {
			numGroups *= shape[i]
		}
	}

	resultIdx := 0
	for group := 0; group < numGroups; group++ {
		baseIdx := 0
		remaining := group
		for i := 0; i < len(shape); i++ {
			if i == dim {
				continue
			}
			coord := remaining % shape[i]
			remaining /= shape[i]
			baseIdx += coord * strides[i]
		}

		maxVal := data[baseIdx]
		maxIdx := int32(0)
		for i := 1; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			if data[idx] > maxVal {
				maxVal = data[idx]
				maxIdx = int32(i)
			}
		}

		result[resultIdx] = maxIdx
		resultIdx++
	}
}

func argmaxInt64(data []int64, result []int32, shape tensor.Shape, dim int) {
	strides := shape.ComputeStrides()
	dimSize := shape[dim]
	dimStride := strides[dim]

	numGroups := 1
	for i := range shape {
		if i != dim {
			numGroups *= shape[i]
		}
	}

	resultIdx := 0
	for group := 0; group < numGroups; group++ {
		baseIdx := 0
		remaining := group
		for i := 0; i < len(shape); i++ {
			if i == dim {
				continue
			}
			coord := remaining % shape[i]
			remaining /= shape[i]
			baseIdx += coord * strides[i]
		}

		maxVal := data[baseIdx]
		maxIdx := int32(0)
		for i := 1; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			if data[idx] > maxVal {
				maxVal = data[idx]
				maxIdx = int32(i)
			}
		}

		result[resultIdx] = maxIdx
		resultIdx++
	}
}
