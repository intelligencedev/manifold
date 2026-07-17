package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Expand broadcasts the tensor to a new shape.
func (cpu *CPUBackend) Expand(x *tensor.RawTensor, newShape tensor.Shape) *tensor.RawTensor {
	// Validate that newShape is compatible with x.Shape()
	xShape := x.Shape()

	// newShape must have >= dimensions
	if len(newShape) < len(xShape) {
		panic(fmt.Sprintf("expand: new shape %v has fewer dimensions than input shape %v",
			newShape, xShape))
	}

	// Align shapes from the right (last dimension)
	// Check that each dimension is either:
	// 1. Equal to new dimension
	// 2. Equal to 1 (can be broadcast)
	offset := len(newShape) - len(xShape)
	for i := 0; i < len(xShape); i++ {
		xDim := xShape[i]
		newDim := newShape[offset+i]
		if xDim != 1 && xDim != newDim {
			panic(fmt.Sprintf("expand: cannot expand dimension %d from %d to %d",
				i, xDim, newDim))
		}
	}

	result, err := tensor.NewRaw(newShape, x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("expand: %v", err))
	}

	// Perform broadcasting
	expandBroadcast(result, x, newShape)

	return result
}

func expandBroadcast(result, x *tensor.RawTensor, outShape tensor.Shape) {
	// Naive implementation - iterate over output indices and map to input
	xShape := x.Shape()
	offset := len(outShape) - len(xShape)

	totalSize := 1
	for _, dim := range outShape {
		totalSize *= dim
	}

	// Compute output strides
	outStrides := make([]int, len(outShape))
	outStrides[len(outShape)-1] = 1
	for i := len(outShape) - 2; i >= 0; i-- {
		outStrides[i] = outStrides[i+1] * outShape[i+1]
	}

	// Compute input strides
	xStrides := make([]int, len(xShape))
	xStrides[len(xShape)-1] = 1
	for i := len(xShape) - 2; i >= 0; i-- {
		xStrides[i] = xStrides[i+1] * xShape[i+1]
	}

	// Type-specific implementation
	switch x.DType() {
	case tensor.Float32:
		expandBroadcastFloat32(result, x, outShape, xShape, offset, outStrides, xStrides, totalSize)
	case tensor.Float64:
		expandBroadcastFloat64(result, x, outShape, xShape, offset, outStrides, xStrides, totalSize)
	case tensor.Int32:
		expandBroadcastInt32(result, x, outShape, xShape, offset, outStrides, xStrides, totalSize)
	case tensor.Int64:
		expandBroadcastInt64(result, x, outShape, xShape, offset, outStrides, xStrides, totalSize)
	case tensor.Bool:
		expandBroadcastBool(result, x, outShape, xShape, offset, outStrides, xStrides, totalSize)
	default:
		panic(fmt.Sprintf("expand: unsupported dtype %v", x.DType()))
	}
}

func expandBroadcastFloat32(result, x *tensor.RawTensor, outShape, xShape tensor.Shape,
	offset int, outStrides, xStrides []int, totalSize int) {
	src := x.AsFloat32()
	dst := result.AsFloat32()

	for outIdx := 0; outIdx < totalSize; outIdx++ {
		// Convert linear index to multi-dim coordinates
		coords := make([]int, len(outShape))
		remaining := outIdx
		for i := 0; i < len(outShape); i++ {
			coords[i] = remaining / outStrides[i]
			remaining %= outStrides[i]
		}

		// Map to input index
		inIdx := 0
		for i := 0; i < len(xShape); i++ {
			outDim := offset + i
			xDim := xShape[i]
			coord := coords[outDim]
			if xDim == 1 {
				coord = 0 // Broadcast dimension
			}
			inIdx += coord * xStrides[i]
		}

		dst[outIdx] = src[inIdx]
	}
}

func expandBroadcastFloat64(result, x *tensor.RawTensor, outShape, xShape tensor.Shape,
	offset int, outStrides, xStrides []int, totalSize int) {
	src := x.AsFloat64()
	dst := result.AsFloat64()

	for outIdx := 0; outIdx < totalSize; outIdx++ {
		coords := make([]int, len(outShape))
		remaining := outIdx
		for i := 0; i < len(outShape); i++ {
			coords[i] = remaining / outStrides[i]
			remaining %= outStrides[i]
		}

		inIdx := 0
		for i := 0; i < len(xShape); i++ {
			outDim := offset + i
			xDim := xShape[i]
			coord := coords[outDim]
			if xDim == 1 {
				coord = 0
			}
			inIdx += coord * xStrides[i]
		}

		dst[outIdx] = src[inIdx]
	}
}

func expandBroadcastInt32(result, x *tensor.RawTensor, outShape, xShape tensor.Shape,
	offset int, outStrides, xStrides []int, totalSize int) {
	src := x.AsInt32()
	dst := result.AsInt32()

	for outIdx := 0; outIdx < totalSize; outIdx++ {
		coords := make([]int, len(outShape))
		remaining := outIdx
		for i := 0; i < len(outShape); i++ {
			coords[i] = remaining / outStrides[i]
			remaining %= outStrides[i]
		}

		inIdx := 0
		for i := 0; i < len(xShape); i++ {
			outDim := offset + i
			xDim := xShape[i]
			coord := coords[outDim]
			if xDim == 1 {
				coord = 0
			}
			inIdx += coord * xStrides[i]
		}

		dst[outIdx] = src[inIdx]
	}
}

func expandBroadcastInt64(result, x *tensor.RawTensor, outShape, xShape tensor.Shape,
	offset int, outStrides, xStrides []int, totalSize int) {
	src := x.AsInt64()
	dst := result.AsInt64()

	for outIdx := 0; outIdx < totalSize; outIdx++ {
		coords := make([]int, len(outShape))
		remaining := outIdx
		for i := 0; i < len(outShape); i++ {
			coords[i] = remaining / outStrides[i]
			remaining %= outStrides[i]
		}

		inIdx := 0
		for i := 0; i < len(xShape); i++ {
			outDim := offset + i
			xDim := xShape[i]
			coord := coords[outDim]
			if xDim == 1 {
				coord = 0
			}
			inIdx += coord * xStrides[i]
		}

		dst[outIdx] = src[inIdx]
	}
}

func expandBroadcastBool(result, x *tensor.RawTensor, outShape, xShape tensor.Shape,
	offset int, outStrides, xStrides []int, totalSize int) {
	src := x.AsBool()
	dst := result.AsBool()

	for outIdx := 0; outIdx < totalSize; outIdx++ {
		coords := make([]int, len(outShape))
		remaining := outIdx
		for i := 0; i < len(outShape); i++ {
			coords[i] = remaining / outStrides[i]
			remaining %= outStrides[i]
		}

		inIdx := 0
		for i := 0; i < len(xShape); i++ {
			outDim := offset + i
			xDim := xShape[i]
			coord := coords[outDim]
			if xDim == 1 {
				coord = 0
			}
			inIdx += coord * xStrides[i]
		}

		dst[outIdx] = src[inIdx]
	}
}
