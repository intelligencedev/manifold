package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// MaxPool2DOp records a max pooling operation for autodiff.
//
// Forward:
//
//	output[n,c,h,w] = max(input[n,c,h*stride+kh,w*stride+kw] for kh,kw in kernel)
//
// Backward:
//   - Input gradient: Gradients flow only to positions that had the max value
//   - For each output position, only one input position receives gradient
//   - All other positions in pooling window receive zero gradient
//
// Example (2x2 pool, stride=2):
//
//	Input:  [[1, 2],  Output: [4]  Input Grad: [[0, 0],
//	         [3, 4]]                             [0, grad]]
//
// Unlike Conv2D which has learnable parameters, MaxPool2D only has input gradients.
type MaxPool2DOp struct {
	input      *tensor.RawTensor
	output     *tensor.RawTensor
	maxIndices []int // Flat indices of max positions for gradient routing
	kernelSize int
	stride     int
}

// NewMaxPool2DOp creates a new MaxPool2D operation.
//
// CRITICAL: Must compute and store max indices during forward pass!
// Without max indices, backward pass cannot route gradients correctly.
func NewMaxPool2DOp(input, output *tensor.RawTensor, kernelSize, stride int) *MaxPool2DOp {
	// Compute max indices for gradient routing
	maxIndices := computeMaxIndices(input, output, kernelSize, stride)

	return &MaxPool2DOp{
		input:      input,
		output:     output,
		maxIndices: maxIndices,
		kernelSize: kernelSize,
		stride:     stride,
	}
}

// computeMaxIndices finds which input position had max value for each output position.
func computeMaxIndices(input, output *tensor.RawTensor, kernelSize, stride int) []int {
	inputShape := input.Shape()
	outputShape := output.Shape()

	N := inputShape[0]
	C := inputShape[1]
	H := inputShape[2]
	W := inputShape[3]
	HOut := outputShape[2]
	WOut := outputShape[3]

	numOutputs := N * C * HOut * WOut
	maxIndices := make([]int, numOutputs)

	poolDims := &tensor.PoolDims{
		N: N, C: C, H: H, W: W,
		KH: kernelSize, KW: kernelSize,
		HOut: HOut, WOut: WOut,
		Stride: stride,
	}

	// Compute max indices based on dtype
	switch input.DType() {
	case tensor.Float32:
		computeMaxIndicesFloat32(maxIndices, input, poolDims)
	case tensor.Float64:
		computeMaxIndicesFloat64(maxIndices, input, poolDims)
	default:
		panic("MaxPool2D: unsupported dtype")
	}

	return maxIndices
}

// computeMaxIndicesFloat32 finds max positions for float32 tensors.
func computeMaxIndicesFloat32(maxIndices []int, input *tensor.RawTensor, dims *tensor.PoolDims) {
	inputData := input.AsFloat32()

	N := dims.N
	C := dims.C
	H := dims.H
	W := dims.W
	HOut := dims.HOut
	WOut := dims.WOut
	kernelSize := dims.KH
	stride := dims.Stride

	outIdx := 0
	for n := 0; n < N; n++ {
		for c := 0; c < C; c++ {
			for outH := 0; outH < HOut; outH++ {
				for outW := 0; outW < WOut; outW++ {
					hStart := outH * stride
					wStart := outW * stride

					// Find max position in pooling window
					maxVal := float32(-1e38)
					maxPos := 0

					for kh := 0; kh < kernelSize; kh++ {
						for kw := 0; kw < kernelSize; kw++ {
							h := hStart + kh
							w := wStart + kw

							inputIdx := ((n*C+c)*H+h)*W + w
							val := inputData[inputIdx]

							if val > maxVal {
								maxVal = val
								maxPos = inputIdx
							}
						}
					}

					maxIndices[outIdx] = maxPos
					outIdx++
				}
			}
		}
	}
}

// computeMaxIndicesFloat64 finds max positions for float64 tensors.
func computeMaxIndicesFloat64(maxIndices []int, input *tensor.RawTensor, dims *tensor.PoolDims) {
	inputData := input.AsFloat64()

	N := dims.N
	C := dims.C
	H := dims.H
	W := dims.W
	HOut := dims.HOut
	WOut := dims.WOut
	kernelSize := dims.KH
	stride := dims.Stride

	outIdx := 0
	for n := 0; n < N; n++ {
		for c := 0; c < C; c++ {
			for outH := 0; outH < HOut; outH++ {
				for outW := 0; outW < WOut; outW++ {
					hStart := outH * stride
					wStart := outW * stride

					// Find max position in pooling window
					maxVal := float64(-1e308)
					maxPos := 0

					for kh := 0; kh < kernelSize; kh++ {
						for kw := 0; kw < kernelSize; kw++ {
							h := hStart + kh
							w := wStart + kw

							inputIdx := ((n*C+c)*H+h)*W + w
							val := inputData[inputIdx]

							if val > maxVal {
								maxVal = val
								maxPos = inputIdx
							}
						}
					}

					maxIndices[outIdx] = maxPos
					outIdx++
				}
			}
		}
	}
}

// Inputs returns the input tensors.
func (op *MaxPool2DOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *MaxPool2DOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes gradients for MaxPool2D.
//
// Gradient routing:
//  1. Initialize input gradient to zeros
//  2. For each output gradient value
//  3. Route it to the input position that had the max value (stored in maxIndices)
//  4. All other positions in pooling window remain zero
//
// This implements the subgradient of the max function:
//
//	∂max(x_i)/∂x_j = 1 if j = argmax(x_i), else 0
//
// This is pure orchestration - delegates computation to backend.
//
// References:
//   - Burn framework: crates/burn-autodiff/src/ops/module.rs (max_pool2d_backward)
func (op *MaxPool2DOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Orchestration: delegate to backend
	inputGrad := backend.MaxPool2DBackward(op.input, outputGrad, op.maxIndices, op.kernelSize, op.stride)

	return []*tensor.RawTensor{inputGrad}
}
