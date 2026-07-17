package cpu

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// Softmax computes softmax along the specified dimension.
// Softmax(x_i) = exp(x_i) / sum(exp(x_j)) for all j in dimension.
func (cpu *CPUBackend) Softmax(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	// Normalize dimension
	if dim < 0 {
		dim = ndim + dim
	}
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("softmax: dimension %d out of range for tensor of rank %d", dim, ndim))
	}

	result, err := tensor.NewRaw(shape, x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("softmax: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		softmaxFloat32(result, x, dim)
	case tensor.Float64:
		softmaxFloat64(result, x, dim)
	default:
		panic(fmt.Sprintf("softmax: unsupported dtype %s (only float32/float64 supported)", x.DType()))
	}

	return result
}

func softmaxFloat32(result, x *tensor.RawTensor, dim int) {
	src := x.AsFloat32()
	dst := result.AsFloat32()
	shape := x.Shape()

	// Compute strides
	strides := make([]int, len(shape))
	strides[len(shape)-1] = 1
	for i := len(shape) - 2; i >= 0; i-- {
		strides[i] = strides[i+1] * shape[i+1]
	}

	dimSize := shape[dim]
	dimStride := strides[dim]

	// Number of "rows" (groups of elements that share softmax computation)
	numRows := 1
	for i := range shape {
		if i != dim {
			numRows *= shape[i]
		}
	}

	// For each row
	for row := 0; row < numRows; row++ {
		// Compute base index for this row
		baseIdx := 0
		remaining := row
		for i := 0; i < len(shape); i++ {
			if i == dim {
				continue
			}
			coord := remaining % shape[i]
			remaining /= shape[i]
			baseIdx += coord * strides[i]
		}

		// Find max for numerical stability
		maxVal := float32(math.Inf(-1))
		for i := 0; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			if src[idx] > maxVal {
				maxVal = src[idx]
			}
		}

		// Compute exp(x - max) and sum
		var sum float32
		for i := 0; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			expVal := float32(math.Exp(float64(src[idx] - maxVal)))
			dst[idx] = expVal
			sum += expVal
		}

		// Normalize
		for i := 0; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			dst[idx] /= sum
		}
	}
}

func softmaxFloat64(result, x *tensor.RawTensor, dim int) {
	src := x.AsFloat64()
	dst := result.AsFloat64()
	shape := x.Shape()

	// Compute strides
	strides := make([]int, len(shape))
	strides[len(shape)-1] = 1
	for i := len(shape) - 2; i >= 0; i-- {
		strides[i] = strides[i+1] * shape[i+1]
	}

	dimSize := shape[dim]
	dimStride := strides[dim]

	// Number of "rows"
	numRows := 1
	for i := range shape {
		if i != dim {
			numRows *= shape[i]
		}
	}

	for row := 0; row < numRows; row++ {
		baseIdx := 0
		remaining := row
		for i := 0; i < len(shape); i++ {
			if i == dim {
				continue
			}
			coord := remaining % shape[i]
			remaining /= shape[i]
			baseIdx += coord * strides[i]
		}

		// Find max
		maxVal := math.Inf(-1)
		for i := 0; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			if src[idx] > maxVal {
				maxVal = src[idx]
			}
		}

		// Compute exp and sum
		var sum float64
		for i := 0; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			expVal := math.Exp(src[idx] - maxVal)
			dst[idx] = expVal
			sum += expVal
		}

		// Normalize
		for i := 0; i < dimSize; i++ {
			idx := baseIdx + i*dimStride
			dst[idx] /= sum
		}
	}
}
