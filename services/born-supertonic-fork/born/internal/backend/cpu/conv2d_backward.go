package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Conv2DInputBackward computes gradient w.r.t. input using transposed convolution.
//
// Algorithm: Transposed convolution (full convolution).
//   - For each input position (n, c_in, h, w):
//   - Sum contributions from all output positions that used this input
//   - Each contribution is: grad[n, c_out, h_out, w_out] * kernel[c_out, c_in, kh, kw]
//
// References:
//   - Burn framework: crates/burn-autodiff/src/ops/module.rs (conv2d_x_backward)
//   - "A guide to convolution arithmetic for deep learning" (Dumoulin & Visin, 2016)
//
//nolint:dupl // Intentional duplication with Conv2DKernelBackward (different operations)
func (cpu *CPUBackend) Conv2DInputBackward(input, kernel, grad *tensor.RawTensor, stride, padding int) *tensor.RawTensor {
	// Extract shapes
	inputShape := input.Shape()
	kernelShape := kernel.Shape()
	gradShape := grad.Shape()

	N := inputShape[0]
	CIn := inputShape[1]
	H := inputShape[2]
	W := inputShape[3]
	COut := kernelShape[0]
	KH := kernelShape[2]
	KW := kernelShape[3]
	HOut := gradShape[2]
	WOut := gradShape[3]

	// Create input gradient tensor
	inputGrad, err := tensor.NewRaw(tensor.Shape{N, CIn, H, W}, grad.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("Conv2DInputBackward: failed to create gradient tensor: %v", err))
	}

	convDims := &ConvDims{
		N: N, CIn: CIn, H: H, W: W,
		COut: COut, KH: KH, KW: KW,
		HOut: HOut, WOut: WOut,
		Stride: stride, Padding: padding,
	}

	// Dispatch by dtype and stride (stride specialization for compiler optimization)
	switch grad.DType() {
	case tensor.Float32:
		if stride == 1 && padding == 0 {
			conv2dInputBackwardFloat32Stride1NoPad(
				inputGrad, grad, kernel,
				convDims,
			)
		} else {
			conv2dInputBackwardFloat32(
				inputGrad, grad, kernel,
				convDims,
			)
		}
	case tensor.Float64:
		if stride == 1 && padding == 0 {
			conv2dInputBackwardFloat64Stride1NoPad(
				inputGrad, grad, kernel,
				convDims,
			)
		} else {
			conv2dInputBackwardFloat64(
				inputGrad, grad, kernel,
				convDims,
			)
		}
	default:
		panic("Conv2DInputBackward: unsupported dtype")
	}

	return inputGrad
}

// conv2dInputBackwardFloat32 computes input gradient for float32.
//
//nolint:dupl // Intentional duplication for float32/float64; high complexity inherent to convolution backprop.
func conv2dInputBackwardFloat32(
	inputGrad, grad, kernel *tensor.RawTensor,
	dims *ConvDims,
) {
	inputGradData := inputGrad.AsFloat32()
	gradData := grad.AsFloat32()
	kernelData := kernel.AsFloat32()

	n := dims.N
	cIn := dims.CIn
	h := dims.H
	w := dims.W
	cOut := dims.COut
	kH := dims.KH
	kW := dims.KW
	hOut := dims.HOut
	wOut := dims.WOut
	stride := dims.Stride
	padding := dims.Padding

	// Initialize to zero
	for i := range inputGradData {
		inputGradData[i] = 0.0
	}

	// For each batch
	for batch := 0; batch < n; batch++ {
		// Pre-slice batch planes
		inputGradBatchOffset := batch * cIn * h * w
		inputGradBatch := inputGradData[inputGradBatchOffset : inputGradBatchOffset+cIn*h*w]

		gradBatchOffset := batch * cOut * hOut * wOut
		gradBatch := gradData[gradBatchOffset : gradBatchOffset+cOut*hOut*wOut]

		// For each output gradient position
		for outH := 0; outH < hOut; outH++ {
			for outW := 0; outW < wOut; outW++ {
				// For each output channel
				for outChan := 0; outChan < cOut; outChan++ {
					gradIdx := outChan*hOut*wOut + outH*wOut + outW
					gradVal := gradBatch[gradIdx]

					// Pre-slice kernel for this output channel
					kernelCOutOffset := outChan * cIn * kH * kW
					kernelCOut := kernelData[kernelCOutOffset : kernelCOutOffset+cIn*kH*kW]

					// Distribute this gradient to all input positions via helper.
					for inChan := 0; inChan < cIn; inChan++ {
						// Pre-slice input gradient channel
						inputGradCInOffset := inChan * h * w
						inputGradCIn := inputGradBatch[inputGradCInOffset : inputGradCInOffset+h*w]

						// Pre-slice kernel for this input channel
						kernelCInOffset := inChan * kH * kW
						kernelCIn := kernelCOut[kernelCInOffset : kernelCInOffset+kH*kW]

						accumulateInputGradFloat32(
							inputGradCIn, kernelCIn,
							gradVal,
							kH, kW,
							outH, outW,
							h, w,
							stride, padding,
						)
					}
				}
			}
		}
	}
}

// conv2dInputBackwardFloat64 computes input gradient for float64.
//
//nolint:dupl // Intentional duplication for float32/float64; high complexity inherent to convolution backprop.
func conv2dInputBackwardFloat64(
	inputGrad, grad, kernel *tensor.RawTensor,
	dims *ConvDims,
) {
	inputGradData := inputGrad.AsFloat64()
	gradData := grad.AsFloat64()
	kernelData := kernel.AsFloat64()

	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut
	stride := dims.Stride
	padding := dims.Padding

	for i := range inputGradData {
		inputGradData[i] = 0.0
	}

	for n := 0; n < N; n++ {
		// Pre-slice batch planes
		inputGradBatchOffset := n * CIn * H * W
		inputGradBatch := inputGradData[inputGradBatchOffset : inputGradBatchOffset+CIn*H*W]

		gradBatchOffset := n * COut * HOut * WOut
		gradBatch := gradData[gradBatchOffset : gradBatchOffset+COut*HOut*WOut]

		for outH := 0; outH < HOut; outH++ {
			for outW := 0; outW < WOut; outW++ {
				for cOut := 0; cOut < COut; cOut++ {
					gradIdx := cOut*HOut*WOut + outH*WOut + outW
					gradVal := gradBatch[gradIdx]

					// Pre-slice kernel for this output channel
					kernelCOutOffset := cOut * CIn * KH * KW
					kernelCOut := kernelData[kernelCOutOffset : kernelCOutOffset+CIn*KH*KW]

					for cIn := 0; cIn < CIn; cIn++ {
						// Pre-slice input gradient channel
						inputGradCInOffset := cIn * H * W
						inputGradCIn := inputGradBatch[inputGradCInOffset : inputGradCInOffset+H*W]

						// Pre-slice kernel for this input channel
						kernelCInOffset := cIn * KH * KW
						kernelCIn := kernelCOut[kernelCInOffset : kernelCInOffset+KH*KW]

						accumulateInputGradFloat64(
							inputGradCIn, kernelCIn,
							gradVal,
							KH, KW,
							outH, outW,
							H, W,
							stride, padding,
						)
					}
				}
			}
		}
	}
}

// conv2dInputBackwardFloat32Stride1NoPad is optimized for stride=1, padding=0.
// Compiler can better optimize this with hardcoded stride=1 (loop unrolling, SIMD).
//
//nolint:dupl // Intentional duplication for float32/float64; high complexity inherent to convolution backprop.
func conv2dInputBackwardFloat32Stride1NoPad(
	inputGrad, grad, kernel *tensor.RawTensor,
	dims *ConvDims,
) {
	inputGradData := inputGrad.AsFloat32()
	gradData := grad.AsFloat32()
	kernelData := kernel.AsFloat32()

	n := dims.N
	cIn := dims.CIn
	h := dims.H
	w := dims.W
	cOut := dims.COut
	kH := dims.KH
	kW := dims.KW
	hOut := dims.HOut
	wOut := dims.WOut

	// Initialize to zero
	for i := range inputGradData {
		inputGradData[i] = 0.0
	}

	// For each batch
	for batch := 0; batch < n; batch++ {
		inputGradBatchOffset := batch * cIn * h * w
		inputGradBatch := inputGradData[inputGradBatchOffset : inputGradBatchOffset+cIn*h*w]

		gradBatchOffset := batch * cOut * hOut * wOut
		gradBatch := gradData[gradBatchOffset : gradBatchOffset+cOut*hOut*wOut]

		// For each output gradient position
		for outH := 0; outH < hOut; outH++ {
			for outW := 0; outW < wOut; outW++ {
				// For each output channel
				for outChan := 0; outChan < cOut; outChan++ {
					gradIdx := outChan*hOut*wOut + outH*wOut + outW
					gradVal := gradBatch[gradIdx]

					kernelCOutOffset := outChan * cIn * kH * kW
					kernelCOut := kernelData[kernelCOutOffset : kernelCOutOffset+cIn*kH*kW]

					// Distribute this gradient to all input positions via helper.
					for inChan := 0; inChan < cIn; inChan++ {
						inputGradCInOffset := inChan * h * w
						inputGradCIn := inputGradBatch[inputGradCInOffset : inputGradCInOffset+h*w]

						kernelCInOffset := inChan * kH * kW
						kernelCIn := kernelCOut[kernelCInOffset : kernelCInOffset+kH*kW]

						accumulateInputGradFloat32Stride1NoPad(
							inputGradCIn, kernelCIn,
							gradVal,
							kH, kW,
							outH, outW, w,
						)
					}
				}
			}
		}
	}
}

// conv2dInputBackwardFloat64Stride1NoPad is optimized for stride=1, padding=0.
// Compiler can better optimize this with hardcoded stride=1 (loop unrolling, SIMD).
//
//nolint:dupl // Intentional duplication for float32/float64; high complexity inherent to convolution backprop.
func conv2dInputBackwardFloat64Stride1NoPad(
	inputGrad, grad, kernel *tensor.RawTensor,
	dims *ConvDims,
) {
	inputGradData := inputGrad.AsFloat64()
	gradData := grad.AsFloat64()
	kernelData := kernel.AsFloat64()

	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut

	for i := range inputGradData {
		inputGradData[i] = 0.0
	}

	for n := 0; n < N; n++ {
		inputGradBatchOffset := n * CIn * H * W
		inputGradBatch := inputGradData[inputGradBatchOffset : inputGradBatchOffset+CIn*H*W]

		gradBatchOffset := n * COut * HOut * WOut
		gradBatch := gradData[gradBatchOffset : gradBatchOffset+COut*HOut*WOut]

		for outH := 0; outH < HOut; outH++ {
			for outW := 0; outW < WOut; outW++ {
				for cOut := 0; cOut < COut; cOut++ {
					gradIdx := cOut*HOut*WOut + outH*WOut + outW
					gradVal := gradBatch[gradIdx]

					kernelCOutOffset := cOut * CIn * KH * KW
					kernelCOut := kernelData[kernelCOutOffset : kernelCOutOffset+CIn*KH*KW]

					for cIn := 0; cIn < CIn; cIn++ {
						inputGradCInOffset := cIn * H * W
						inputGradCIn := inputGradBatch[inputGradCInOffset : inputGradCInOffset+H*W]

						kernelCInOffset := cIn * KH * KW
						kernelCIn := kernelCOut[kernelCInOffset : kernelCInOffset+KH*KW]

						accumulateInputGradFloat64Stride1NoPad(
							inputGradCIn, kernelCIn,
							gradVal,
							KH, KW,
							outH, outW, W,
						)
					}
				}
			}
		}
	}
}

// Conv2DKernelBackward computes gradient w.r.t. kernel.
//
// Algorithm: Convolution of input with grad.
//   - For each kernel position (c_out, c_in, kh, kw):
//   - Sum over all batch samples and output positions
//   - Each contribution is: input[n, c_in, h, w] * grad[n, c_out, h_out, w_out]
//   - Where h = h_out * stride - padding + kh, w = w_out * stride - padding + kw
//
// References:
//   - Burn framework: crates/burn-autodiff/src/ops/module.rs (conv2d_weight_backward)
//
//nolint:dupl // Intentional duplication with Conv2DInputBackward (different operations)
func (cpu *CPUBackend) Conv2DKernelBackward(input, kernel, grad *tensor.RawTensor, stride, padding int) *tensor.RawTensor {
	inputShape := input.Shape()
	kernelShape := kernel.Shape()
	gradShape := grad.Shape()

	N := inputShape[0]
	CIn := inputShape[1]
	H := inputShape[2]
	W := inputShape[3]
	COut := kernelShape[0]
	KH := kernelShape[2]
	KW := kernelShape[3]
	HOut := gradShape[2]
	WOut := gradShape[3]

	kernelGrad, err := tensor.NewRaw(tensor.Shape{COut, CIn, KH, KW}, grad.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("Conv2DKernelBackward: failed to create gradient tensor: %v", err))
	}

	convDims := &ConvDims{
		N: N, CIn: CIn, H: H, W: W,
		COut: COut, KH: KH, KW: KW,
		HOut: HOut, WOut: WOut,
		Stride: stride, Padding: padding,
	}

	// Dispatch by dtype and stride (stride specialization for compiler optimization)
	switch grad.DType() {
	case tensor.Float32:
		if stride == 1 && padding == 0 {
			conv2dKernelBackwardFloat32Stride1NoPad(
				kernelGrad, grad, input,
				convDims,
			)
		} else {
			conv2dKernelBackwardFloat32(
				kernelGrad, grad, input,
				convDims,
			)
		}
	case tensor.Float64:
		if stride == 1 && padding == 0 {
			conv2dKernelBackwardFloat64Stride1NoPad(
				kernelGrad, grad, input,
				convDims,
			)
		} else {
			conv2dKernelBackwardFloat64(
				kernelGrad, grad, input,
				convDims,
			)
		}
	default:
		panic("Conv2DKernelBackward: unsupported dtype")
	}

	return kernelGrad
}

// conv2dKernelBackwardFloat32 computes kernel gradient for float32.
//
//nolint:dupl // Intentional duplication for float32/float64; high complexity inherent to convolution backprop.
func conv2dKernelBackwardFloat32(
	kernelGrad, grad, input *tensor.RawTensor,
	dims *ConvDims,
) {
	kernelGradData := kernelGrad.AsFloat32()
	gradData := grad.AsFloat32()
	inputData := input.AsFloat32()

	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut
	stride := dims.Stride
	padding := dims.Padding

	// Initialize to zero
	for i := range kernelGradData {
		kernelGradData[i] = 0.0
	}

	// For each kernel weight — accumulate via helper.
	for cOut := 0; cOut < COut; cOut++ {
		for cIn := 0; cIn < CIn; cIn++ {
			for kh := 0; kh < KH; kh++ {
				for kw := 0; kw < KW; kw++ {
					sum := kernelGradSumFloat32(
						inputData, gradData,
						N, CIn, H, W, COut, HOut, WOut,
						cOut, cIn, kh, kw,
						stride, padding,
					)
					kernelIdx := cOut*CIn*KH*KW + cIn*KH*KW + kh*KW + kw
					kernelGradData[kernelIdx] = sum
				}
			}
		}
	}
}

// conv2dKernelBackwardFloat64 computes kernel gradient for float64.
//
//nolint:dupl // Intentional duplication for float32/float64; high complexity inherent to convolution backprop.
func conv2dKernelBackwardFloat64(
	kernelGrad, grad, input *tensor.RawTensor,
	dims *ConvDims,
) {
	kernelGradData := kernelGrad.AsFloat64()
	gradData := grad.AsFloat64()
	inputData := input.AsFloat64()

	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut
	stride := dims.Stride
	padding := dims.Padding

	for i := range kernelGradData {
		kernelGradData[i] = 0.0
	}

	// For each kernel weight — accumulate via helper.
	for cOut := 0; cOut < COut; cOut++ {
		for cIn := 0; cIn < CIn; cIn++ {
			for kh := 0; kh < KH; kh++ {
				for kw := 0; kw < KW; kw++ {
					sum := kernelGradSumFloat64(
						inputData, gradData,
						N, CIn, H, W, COut, HOut, WOut,
						cOut, cIn, kh, kw,
						stride, padding,
					)
					kernelIdx := cOut*CIn*KH*KW + cIn*KH*KW + kh*KW + kw
					kernelGradData[kernelIdx] = sum
				}
			}
		}
	}
}

// conv2dKernelBackwardFloat32Stride1NoPad is optimized for stride=1, padding=0.
// Compiler can better optimize this with hardcoded stride=1 (loop unrolling, SIMD).
//
//nolint:dupl // Intentional duplication for float32/float64; high complexity inherent to convolution backprop.
func conv2dKernelBackwardFloat32Stride1NoPad(
	kernelGrad, grad, input *tensor.RawTensor,
	dims *ConvDims,
) {
	kernelGradData := kernelGrad.AsFloat32()
	gradData := grad.AsFloat32()
	inputData := input.AsFloat32()

	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut

	// Initialize to zero
	for i := range kernelGradData {
		kernelGradData[i] = 0.0
	}

	// For each kernel weight — accumulate via helper (stride=1, padding=0 fast path).
	for cOut := 0; cOut < COut; cOut++ {
		for cIn := 0; cIn < CIn; cIn++ {
			for kh := 0; kh < KH; kh++ {
				for kw := 0; kw < KW; kw++ {
					sum := kernelGradSumFloat32Stride1NoPad(
						inputData, gradData,
						N, CIn, H, W, COut, HOut, WOut,
						cOut, cIn, kh, kw,
					)
					kernelIdx := cOut*CIn*KH*KW + cIn*KH*KW + kh*KW + kw
					kernelGradData[kernelIdx] = sum
				}
			}
		}
	}
}

// conv2dKernelBackwardFloat64Stride1NoPad is optimized for stride=1, padding=0.
// Compiler can better optimize this with hardcoded stride=1 (loop unrolling, SIMD).
//
//nolint:dupl // Intentional duplication for float32/float64; high complexity inherent to convolution backprop.
func conv2dKernelBackwardFloat64Stride1NoPad(
	kernelGrad, grad, input *tensor.RawTensor,
	dims *ConvDims,
) {
	kernelGradData := kernelGrad.AsFloat64()
	gradData := grad.AsFloat64()
	inputData := input.AsFloat64()

	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut

	for i := range kernelGradData {
		kernelGradData[i] = 0.0
	}

	// For each kernel weight — accumulate via helper (stride=1, padding=0 fast path).
	for cOut := 0; cOut < COut; cOut++ {
		for cIn := 0; cIn < CIn; cIn++ {
			for kh := 0; kh < KH; kh++ {
				for kw := 0; kw < KW; kw++ {
					sum := kernelGradSumFloat64Stride1NoPad(
						inputData, gradData,
						N, CIn, H, W, COut, HOut, WOut,
						cOut, cIn, kh, kw,
					)
					kernelIdx := cOut*CIn*KH*KW + cIn*KH*KW + kh*KW + kw
					kernelGradData[kernelIdx] = sum
				}
			}
		}
	}
}
