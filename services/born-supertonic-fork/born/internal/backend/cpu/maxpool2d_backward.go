package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// MaxPool2DBackward computes gradient w.r.t. input for MaxPool2D.
//
// Algorithm: Route gradients to max positions.
//   - Gradients flow only to positions that had the max value in forward pass
//   - For each output position, only ONE input position receives gradient
//   - All other positions in pooling window receive zero gradient
//
// Example (2x2 pool, stride=2):
//
//	Input:  [[1, 2],  Output: [4]  Input Grad: [[0, 0],
//	         [3, 4]]                             [0, grad]]
//
// References:
//   - Burn framework: crates/burn-autodiff/src/ops/module.rs (max_pool2d_backward)
//   - CS231n: Backprop for pooling layers
func (cpu *CPUBackend) MaxPool2DBackward(input, grad *tensor.RawTensor, maxIndices []int, kernelSize, stride int) *tensor.RawTensor {
	inputShape := input.Shape()
	gradShape := grad.Shape()

	N := inputShape[0]
	C := inputShape[1]
	H := inputShape[2]
	W := inputShape[3]
	HOut := gradShape[2]
	WOut := gradShape[3]

	// Create input gradient tensor
	inputGrad, err := tensor.NewRaw(inputShape, grad.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("MaxPool2DBackward: failed to create gradient tensor: %v", err))
	}

	// Validate maxIndices length
	expectedLen := N * C * HOut * WOut
	if len(maxIndices) != expectedLen {
		panic(fmt.Sprintf("MaxPool2DBackward: maxIndices length %d != expected %d", len(maxIndices), expectedLen))
	}

	poolDims := &PoolDims{
		N: N, C: C, H: H, W: W,
		KH: kernelSize, KW: kernelSize,
		HOut: HOut, WOut: WOut,
		Stride: stride,
	}

	// Dispatch by dtype
	switch grad.DType() {
	case tensor.Float32:
		maxPool2DBackwardFloat32(
			inputGrad, grad,
			maxIndices,
			poolDims,
		)
	case tensor.Float64:
		maxPool2DBackwardFloat64(
			inputGrad, grad,
			maxIndices,
			poolDims,
		)
	default:
		panic("MaxPool2DBackward: unsupported dtype")
	}

	return inputGrad
}

// maxPool2DBackwardFloat32 routes gradients to max positions for float32.
func maxPool2DBackwardFloat32(
	inputGrad, grad *tensor.RawTensor,
	maxIndices []int,
	dims *PoolDims,
) {
	inputGradData := inputGrad.AsFloat32()
	gradData := grad.AsFloat32()

	N := dims.N
	C := dims.C
	HOut := dims.HOut
	WOut := dims.WOut

	// Initialize to zero
	for i := range inputGradData {
		inputGradData[i] = 0.0
	}

	// Route gradients
	outIdx := 0
	for n := 0; n < N; n++ {
		for c := 0; c < C; c++ {
			for outH := 0; outH < HOut; outH++ {
				for outW := 0; outW < WOut; outW++ {
					// Get max position from forward pass
					maxPos := maxIndices[outIdx]

					// Get gradient value
					gradIdx := n*C*HOut*WOut + c*HOut*WOut + outH*WOut + outW
					gradVal := gradData[gradIdx]

					// Route gradient to max position
					inputGradData[maxPos] += gradVal

					outIdx++
				}
			}
		}
	}
}

// maxPool2DBackwardFloat64 routes gradients to max positions for float64.
func maxPool2DBackwardFloat64(
	inputGrad, grad *tensor.RawTensor,
	maxIndices []int,
	dims *PoolDims,
) {
	inputGradData := inputGrad.AsFloat64()
	gradData := grad.AsFloat64()

	N := dims.N
	C := dims.C
	HOut := dims.HOut
	WOut := dims.WOut

	for i := range inputGradData {
		inputGradData[i] = 0.0
	}

	outIdx := 0
	for n := 0; n < N; n++ {
		for c := 0; c < C; c++ {
			for outH := 0; outH < HOut; outH++ {
				for outW := 0; outW < WOut; outW++ {
					maxPos := maxIndices[outIdx]

					gradIdx := n*C*HOut*WOut + c*HOut*WOut + outH*WOut + outW
					gradVal := gradData[gradIdx]

					inputGradData[maxPos] += gradVal

					outIdx++
				}
			}
		}
	}
}
