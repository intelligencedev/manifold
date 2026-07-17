package ops

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// typedScalar returns a Go value matching the tensor dtype for use with backend scalar ops.
func typedScalar(dtype tensor.DataType, value float64) any {
	switch dtype {
	case tensor.Float32:
		return float32(value)
	case tensor.Float64:
		return value
	default:
		panic(fmt.Sprintf("typedScalar: unsupported dtype %s", dtype))
	}
}

// mulScalarTyped multiplies t by a scalar matched to the tensor's dtype.
func mulScalarTyped(t *tensor.RawTensor, value float64, backend tensor.Backend) *tensor.RawTensor {
	return backend.MulScalar(t, typedScalar(t.DType(), value))
}

// reduceBroadcast reduces a gradient tensor to match the target shape.
// This is necessary when broadcasting was used in the forward pass.
//
// Example:
//
//	Forward: a[3,1] + b[3,4] -> c[3,4]  (a was broadcast along dim 1)
//	Backward: grad_c[3,4] -> grad_a[3,1] (sum along dim 1)
func reduceBroadcast(grad *tensor.RawTensor, targetShape tensor.Shape, backend tensor.Backend) *tensor.RawTensor {
	gradShape := grad.Shape()

	// If shapes already match, clone to avoid aliasing issues
	// (prevents inplace operations from modifying shared gradients)
	if gradShape.Equal(targetShape) {
		return grad.Clone()
	}

	// Handle scalar upstream gradient (empty shape)
	// This happens when gradient flows from scalar loss (e.g., CrossEntropy, MSE)
	// We need to broadcast scalar to target shape
	if len(gradShape) == 0 {
		// Extract scalar value
		var scalarValue float64
		switch grad.DType() {
		case tensor.Float32:
			scalarValue = float64(grad.AsFloat32()[0])
		case tensor.Float64:
			scalarValue = grad.AsFloat64()[0]
		default:
			panic(fmt.Sprintf("reduceBroadcast: unsupported dtype %s for scalar gradient", grad.DType()))
		}

		// Create tensor filled with scalar value in target shape
		return createScalar(targetShape, grad.DType(), scalarValue, grad.Device())
	}

	// Handle scalar target (empty shape)
	if len(targetShape) == 0 {
		// Sum all elements to scalar
		return sumAll(grad, backend)
	}

	// Handle broadcasting reduction
	// NumPy broadcasting aligns shapes from the right
	gradDims := len(gradShape)
	targetDims := len(targetShape)

	// If target has fewer dimensions, sum leading dimensions
	if targetDims < gradDims {
		dimsToSum := gradDims - targetDims
		result := grad
		for i := 0; i < dimsToSum; i++ {
			result = sumAlongDimension(result, 0, backend)
		}
		grad = result
		gradShape = grad.Shape()
	}

	// Now sum along dimensions where target is 1
	result := grad
	for i := 0; i < targetDims; i++ {
		//nolint:gosec // G602: i < targetDims ensures safe index access, targetDims = len(targetShape).
		if targetShape[i] == 1 && gradShape[i] > 1 {
			result = sumAlongDimension(result, i, backend)
		}
	}

	// Reshape if necessary to match target shape exactly
	if !result.Shape().Equal(targetShape) {
		result = backend.Reshape(result, targetShape)
	}

	return result
}

// sumAll sums all elements of a tensor to a scalar.
func sumAll(t *tensor.RawTensor, backend tensor.Backend) *tensor.RawTensor {
	return backend.Sum(t)
}

// sumAlongDimension sums a tensor along the specified dimension, keeping the dim (size 1).
func sumAlongDimension(t *tensor.RawTensor, dim int, backend tensor.Backend) *tensor.RawTensor {
	shape := t.Shape()
	if dim < 0 || dim >= len(shape) {
		panic(fmt.Sprintf("sumAlongDimension: invalid dimension %d for shape %v", dim, shape))
	}

	// SumDim with keepDim=true preserves the reduced dimension as size 1,
	// matching the output shape contract that callers (reduceBroadcast) expect.
	return backend.SumDim(t, dim, true)
}

// negateGradient returns -grad by multiplying by -1.
func negateGradient(grad *tensor.RawTensor, backend tensor.Backend) *tensor.RawTensor {
	return mulScalarTyped(grad, -1.0, backend)
}

// createScalar creates a tensor filled with a scalar value.
// Float32 uses tensor.FullRaw; Float64 fills directly to preserve precision.
func createScalar(shape tensor.Shape, dtype tensor.DataType, value float64, device tensor.Device) *tensor.RawTensor {
	switch dtype {
	case tensor.Float32:
		result, err := tensor.FullRaw(shape, float32(value), dtype, device)
		if err != nil {
			panic(fmt.Sprintf("createScalar: %v", err))
		}

		return result
	case tensor.Float64:
		// FullRaw converts to float32 internally, losing precision.
		// Allocate and fill directly to preserve float64 accuracy.
		result, err := tensor.NewRaw(shape, dtype, device)
		if err != nil {
			panic(fmt.Sprintf("createScalar: %v", err))
		}

		data := result.AsFloat64()
		for i := range data {
			data[i] = value
		}

		return result
	default:
		panic(fmt.Sprintf("createScalar: unsupported dtype %s", dtype))
	}
}
