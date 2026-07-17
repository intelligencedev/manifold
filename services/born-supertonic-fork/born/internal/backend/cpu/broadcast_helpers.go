package cpu

import (
	"github.com/born-ml/born/internal/tensor"
)

// computeBroadcastStridesForShape computes strides for broadcasting a shape to outShape.
// Returns strides where dimensions of size 1 have stride 0 (for broadcasting).
func computeBroadcastStridesForShape(inShape, outShape tensor.Shape) []int {
	outDim := len(outShape)
	strides := make([]int, outDim)

	// Pad input shape with 1s on the left
	inDim := len(inShape)
	offset := outDim - inDim

	// Compute original strides
	origStrides := inShape.ComputeStrides()

	for i := 0; i < outDim; i++ {
		inIdx := i - offset
		switch {
		case inIdx < 0 || inIdx >= inDim:
			// Padded dimension, stride is 0
			strides[i] = 0
		case inShape[inIdx] == 1:
			// Broadcast dimension, stride is 0
			strides[i] = 0
		default:
			// Normal dimension, use original stride
			strides[i] = origStrides[inIdx]
		}
	}

	return strides
}

// computeFlatIndex computes the flat index in the source array for a given output index.
// outStrides: strides of the output shape.
// inStrides: broadcast-adjusted strides of the input shape.
func computeFlatIndex(outIdx int, outStrides, inStrides []int) int {
	ndim := len(outStrides)
	flatIdx := 0

	for i := 0; i < ndim; i++ {
		// Extract coordinate along dimension i
		coord := outIdx / outStrides[i]
		outIdx %= outStrides[i]

		// Add to flat index using input stride
		flatIdx += coord * inStrides[i]
	}

	return flatIdx
}
