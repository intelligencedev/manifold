package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// ConvDims is tensor.ConvDims, defined in the shared tensor package.
type ConvDims = tensor.ConvDims

// Conv2D performs 2D convolution using im2col algorithm.
//
// Input shape: [batch, in_channels, height, width]
// Kernel shape: [out_channels, in_channels, kernel_h, kernel_w]
// Output shape: [batch, out_channels, out_h, out_w]
//
// Parameters:
//   - input: Input tensor [N, C_in, H, W]
//   - kernel: Convolution kernel [C_out, C_in, K_h, K_w]
//   - stride: Stride for convolution (default: 1)
//   - padding: Padding to apply (default: 0)
//
// Algorithm: Im2col
//  1. Transform input patches into columns (im2col)
//  2. Reshape kernel into matrix
//  3. Perform matrix multiplication
//  4. Reshape output to [N, C_out, H_out, W_out]
//
// Im2col is efficient because:
//   - Converts convolution to matmul (highly optimized)
//   - Cache-friendly memory access
//   - Reuses existing matmul code
//
// Reference: "High Performance Convolutional Neural Networks for Document Processing"
// (Chellapilla et al., 2006).
func (cpu *CPUBackend) Conv2D(input, kernel *tensor.RawTensor, stride, padding int) *tensor.RawTensor {
	// Validate input shapes
	inputShape := input.Shape()
	kernelShape := kernel.Shape()

	if len(inputShape) != 4 {
		panic(fmt.Sprintf("conv2d: input must be 4D [N,C,H,W], got %dD", len(inputShape)))
	}
	if len(kernelShape) != 4 {
		panic(fmt.Sprintf("conv2d: kernel must be 4D [C_out,C_in,K_h,K_w], got %dD", len(kernelShape)))
	}

	N := inputShape[0]     // batch size
	CIn := inputShape[1]   // input channels
	H := inputShape[2]     // input height
	W := inputShape[3]     // input width
	COut := kernelShape[0] // output channels
	CInK := kernelShape[1] // kernel input channels (must match CIn)
	KH := kernelShape[2]   // kernel height
	KW := kernelShape[3]   // kernel width

	// Validate channel dimensions
	if CIn != CInK {
		panic(fmt.Sprintf("conv2d: input channels %d != kernel channels %d", CIn, CInK))
	}

	// Compute output dimensions
	// out_h = (H + 2*padding - KH) / stride + 1
	// out_w = (W + 2*padding - KW) / stride + 1
	HOut := (H+2*padding-KH)/stride + 1
	WOut := (W+2*padding-KW)/stride + 1

	if HOut <= 0 || WOut <= 0 {
		panic(fmt.Sprintf("conv2d: invalid output dimensions: out_h=%d, out_w=%d (check stride/padding)", HOut, WOut))
	}

	// Create output tensor [N, C_out, H_out, W_out]
	output, err := tensor.NewRaw(tensor.Shape{N, COut, HOut, WOut}, input.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("conv2d: failed to create output tensor: %v", err))
	}

	convDims := &ConvDims{
		N: N, CIn: CIn, H: H, W: W,
		COut: COut, KH: KH, KW: KW,
		HOut: HOut, WOut: WOut,
		Stride: stride, Padding: padding,
	}

	// Dispatch to type-specific implementation
	switch input.DType() {
	case tensor.Float32:
		conv2dFloat32(output, input, kernel, convDims)
	case tensor.Float64:
		conv2dFloat64(output, input, kernel, convDims)
	default:
		panic(fmt.Sprintf("conv2d: unsupported dtype %s", input.DType()))
	}

	return output
}

// conv2dFloat32 performs Conv2D for float32 using im2col.
//
// Algorithm:
//  1. Im2col: Transform [N, C, H, W] -> [N * H_out * W_out, C * K_h * K_w]
//  2. Reshape kernel: [C_out, C, K_h, K_w] -> [C_out, C * K_h * K_w]
//  3. MatMul: [C_out, C*K_h*K_w] @ [C*K_h*K_w, N*H_out*W_out] -> [C_out, N*H_out*W_out]
//  4. Reshape: [C_out, N*H_out*W_out] -> [N, C_out, H_out, W_out]
//
// Stride specialization: separate path for stride=1 enables compiler optimizations (SIMD).
func conv2dFloat32(output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	// Dispatch to specialized implementation for common case
	if dims.Stride == 1 && dims.Padding == 0 {
		conv2dFloat32Stride1NoPad(output, input, kernel, dims)
		return
	}

	// General case (stride > 1 or padding > 0)
	conv2dFloat32General(output, input, kernel, dims)
}

// conv2dFloat32Stride1NoPad is optimized for stride=1, padding=0 (most common case).
// Compiler can better optimize this with hardcoded stride=1 (loop unrolling, SIMD).
func conv2dFloat32Stride1NoPad(output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	// Pointwise (1x1) fast path: im2col would only transpose the input to
	// [H*W, CIn] for matMulColBufFloat32 to transpose it straight back, so skip
	// im2col, the colBuf transpose, and the output rearrange entirely and run the
	// GEMM directly on the input (kernel[COut,CIn] @ input[CIn,H*W] per batch).
	if dims.KH == 1 && dims.KW == 1 {
		pointwiseConvFloat32(output, input, kernel, dims)
		return
	}

	inputData := input.AsFloat32()
	kernelData := kernel.AsFloat32()
	outputData := output.AsFloat32()

	N := dims.N
	CIn := dims.CIn
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut

	// Step 1: Im2col with stride=1, padding=0. colBuf is recycled from a pool and
	// fully overwritten by im2col, so it needs no zeroing.
	colWidth := CIn * KH * KW
	colHeight := N * HOut * WOut
	colp := poolScratch[float32](&convColPoolF32, colHeight*colWidth)
	colBuf := *colp
	defer convColPoolF32.Put(colp)

	im2colFloat32Stride1NoPad(colBuf, inputData, dims)

	// Step 2: Matrix multiply into pooled scratch (len == len(outputData)).
	// matMulColBufFloat32 writes every element, so the un-zeroed buffer is safe.
	matp := poolScratch[float32](&convOutPoolF32, COut*colHeight)
	matOut := *matp
	defer convOutPoolF32.Put(matp)
	matMulColBufFloat32(matOut, kernelData, colBuf, COut, colHeight, colWidth)

	// Step 3: Rearrange [C_out, N*H_out*W_out] -> [N, C_out, H_out, W_out],
	// permuting the matmul scratch straight into the output (no intermediate copy).
	rearrangeOutputFloat32(outputData, matOut, N, COut, HOut, WOut, colHeight)
}

// pointwiseConvFloat32 computes a 1x1 convolution (stride=1, padding=0) as a
// per-batch matrix multiply output[n] = kernel[COut,CIn] @ input[n][CIn,H*W].
// matmulFloat32 writes the [COut, H*W] product straight into the output buffer,
// which is already the [N, COut, H, W] layout, so no im2col or rearrange is needed.
//
// This relies on matmulFloat32 OVERWRITING its output (it zeroes the buffer up
// front), so each per-batch slice is fully written with no pre-zeroing here. See
// matmulFloat32's documented overwrite contract; an accumulate-mode change there
// would have to be a separate function, not a behavior change.
func pointwiseConvFloat32(output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	inputData := input.AsFloat32()
	kernelData := kernel.AsFloat32()
	outputData := output.AsFloat32()

	cIn, cOut := dims.CIn, dims.COut
	hw := dims.HOut * dims.WOut // == H*W for 1x1 stride=1 padding=0
	inStride := cIn * hw
	outStride := cOut * hw

	for n := 0; n < dims.N; n++ {
		in := inputData[n*inStride : n*inStride+inStride]
		out := outputData[n*outStride : n*outStride+outStride]
		matmulFloat32(out, kernelData, in, cOut, cIn, hw)
	}
}

// conv2dFloat32General handles arbitrary stride and padding.
func conv2dFloat32General(output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	inputData := input.AsFloat32()
	kernelData := kernel.AsFloat32()
	outputData := output.AsFloat32()

	N := dims.N
	CIn := dims.CIn
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut

	// Step 1: Im2col transformation. colBuf: [N*H_out*W_out, C_in*K_h*K_w].
	// Pooled scratch, fully overwritten by im2col, so no zeroing needed.
	colWidth := CIn * KH * KW
	colHeight := N * HOut * WOut
	colp := poolScratch[float32](&convColPoolF32, colHeight*colWidth)
	colBuf := *colp
	defer convColPoolF32.Put(colp)

	im2colFloat32(colBuf, inputData, dims)

	// Step 2: Reshape kernel — already in [C_out, C_in*K_h*K_w] layout (row-major).

	// Step 3: Matrix multiply into pooled scratch (len == len(outputData)).
	// kernel: [C_out, C_in*K_h*K_w] @ colBuf^T -> [C_out, N*H_out*W_out].
	// matMulColBufFloat32 writes every element, so the un-zeroed buffer is safe.
	matp := poolScratch[float32](&convOutPoolF32, COut*colHeight)
	matOut := *matp
	defer convOutPoolF32.Put(matp)
	matMulColBufFloat32(matOut, kernelData, colBuf, COut, colHeight, colWidth)

	// Step 4: Rearrange [C_out, N*H_out*W_out] -> [N, C_out, H_out, W_out],
	// permuting the matmul scratch straight into the output (no intermediate copy).
	rearrangeOutputFloat32(outputData, matOut, N, COut, HOut, WOut, colHeight)
}

// im2colFloat32Stride1NoPad is optimized for stride=1, padding=0.
// Compiler can better optimize with hardcoded stride=1 (no bounds checks for padding).
func im2colFloat32Stride1NoPad(colBuf, inputData []float32, dims *ConvDims) {
	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut

	colWidth := CIn * KH * KW
	colIdx := 0

	for n := 0; n < N; n++ {
		batchOffset := n * CIn * H * W
		batchData := inputData[batchOffset : batchOffset+CIn*H*W]

		for outH := 0; outH < HOut; outH++ {
			for outW := 0; outW < WOut; outW++ {
				// With stride=1, padding=0: hStart = outH, wStart = outW
				rowOffset := colIdx * colWidth
				rowData := colBuf[rowOffset : rowOffset+colWidth]

				bufIdx := 0
				for c := 0; c < CIn; c++ {
					channelOffset := c * H * W
					channelData := batchData[channelOffset : channelOffset+H*W]

					for kh := 0; kh < KH; kh++ {
						h := outH + kh // stride=1: no multiplication
						for kw := 0; kw < KW; kw++ {
							w := outW + kw // stride=1: no multiplication
							// No padding check needed (padding=0 guaranteed)
							rowData[bufIdx] = channelData[h*W+w]
							bufIdx++
						}
					}
				}

				colIdx++
			}
		}
	}
}

// im2colFloat32 transforms input tensor into column matrix (general case).
//
// Input: [N, C, H, W]
// Output: colBuf [N * H_out * W_out, C * K_h * K_w]
//
// Each row of colBuf corresponds to one output position.
// Each column corresponds to one kernel weight.
//
// For each output position (n, out_h, out_w):
//   - Extract the patch from input
//   - Flatten the patch into a row of colBuf
func im2colFloat32(colBuf, inputData []float32, dims *ConvDims) {
	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut
	stride := dims.Stride
	padding := dims.Padding

	colWidth := CIn * KH * KW
	colIdx := 0 // Current row in colBuf

	for n := 0; n < N; n++ {
		// Pre-slice batch: eliminates n*C*H*W bounds check
		batchOffset := n * CIn * H * W
		batchData := inputData[batchOffset : batchOffset+CIn*H*W]

		for outH := 0; outH < HOut; outH++ {
			for outW := 0; outW < WOut; outW++ {
				// For this output position, extract the input patch
				// Top-left corner in input space
				hStart := outH*stride - padding
				wStart := outW*stride - padding

				// Pre-slice output row: eliminates colIdx*colWidth bounds check
				rowOffset := colIdx * colWidth
				rowData := colBuf[rowOffset : rowOffset+colWidth]

				bufIdx := 0
				for c := 0; c < CIn; c++ {
					// Pre-slice channel: eliminates c*H*W bounds check
					channelOffset := c * H * W
					channelData := batchData[channelOffset : channelOffset+H*W]

					for kh := 0; kh < KH; kh++ {
						h := hStart + kh
						for kw := 0; kw < KW; kw++ {
							w := wStart + kw

							// Check bounds (padding)
							if h >= 0 && h < H && w >= 0 && w < W {
								// Valid input position: single bounds check via pre-slice
								rowData[bufIdx] = channelData[h*W+w]
							} else {
								// Out of bounds (padding with zero)
								rowData[bufIdx] = 0.0
							}
							bufIdx++
						}
					}
				}

				colIdx++
			}
		}
	}
}

// conv2dFloat64 performs Conv2D for float64 using im2col.
// Stride specialization: separate path for stride=1 enables compiler optimizations (SIMD).
func conv2dFloat64(output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	// Dispatch to specialized implementation for common case
	if dims.Stride == 1 && dims.Padding == 0 {
		conv2dFloat64Stride1NoPad(output, input, kernel, dims)
		return
	}

	// General case (stride > 1 or padding > 0)
	conv2dFloat64General(output, input, kernel, dims)
}

// conv2dFloat64Stride1NoPad is optimized for stride=1, padding=0 (most common case).
// Compiler can better optimize this with hardcoded stride=1 (loop unrolling, SIMD).
func conv2dFloat64Stride1NoPad(output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	// Pointwise (1x1) fast path: skip im2col/transpose/rearrange (see the float32
	// version for the rationale) and run the GEMM directly on the input.
	if dims.KH == 1 && dims.KW == 1 {
		pointwiseConvFloat64(output, input, kernel, dims)
		return
	}

	inputData := input.AsFloat64()
	kernelData := kernel.AsFloat64()
	outputData := output.AsFloat64()

	N := dims.N
	CIn := dims.CIn
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut

	// Im2col with stride=1, padding=0. Pooled scratch, fully overwritten by
	// im2col, so no zeroing needed.
	colWidth := CIn * KH * KW
	colHeight := N * HOut * WOut
	colp := poolScratch[float64](&convColPoolF64, colHeight*colWidth)
	colBuf := *colp
	defer convColPoolF64.Put(colp)
	im2colFloat64Stride1NoPad(colBuf, inputData, dims)

	// MatMul into pooled scratch (len == len(outputData)); matMulColBufFloat64
	// writes every element, so the un-zeroed buffer is safe.
	matp := poolScratch[float64](&convOutPoolF64, COut*colHeight)
	matOut := *matp
	defer convOutPoolF64.Put(matp)
	matMulColBufFloat64(matOut, kernelData, colBuf, COut, colHeight, colWidth)

	// Rearrange [C_out, N*H_out*W_out] -> [N, C_out, H_out, W_out], permuting the
	// matmul scratch straight into the output (no intermediate copy).
	rearrangeOutputFloat64(outputData, matOut, N, COut, HOut, WOut, colHeight)
}

// pointwiseConvFloat64 is the float64 counterpart of pointwiseConvFloat32 and
// relies on the same matmulFloat64 overwrite contract (no pre-zeroing needed).
func pointwiseConvFloat64(output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	inputData := input.AsFloat64()
	kernelData := kernel.AsFloat64()
	outputData := output.AsFloat64()

	cIn, cOut := dims.CIn, dims.COut
	hw := dims.HOut * dims.WOut // == H*W for 1x1 stride=1 padding=0
	inStride := cIn * hw
	outStride := cOut * hw

	for n := 0; n < dims.N; n++ {
		in := inputData[n*inStride : n*inStride+inStride]
		out := outputData[n*outStride : n*outStride+outStride]
		matmulFloat64(out, kernelData, in, cOut, cIn, hw)
	}
}

// conv2dFloat64General handles arbitrary stride and padding.
func conv2dFloat64General(output, input, kernel *tensor.RawTensor, dims *ConvDims) {
	inputData := input.AsFloat64()
	kernelData := kernel.AsFloat64()
	outputData := output.AsFloat64()

	N := dims.N
	CIn := dims.CIn
	COut := dims.COut
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut

	// Im2col. Pooled scratch, fully overwritten by im2col, so no zeroing needed.
	colWidth := CIn * KH * KW
	colHeight := N * HOut * WOut
	colp := poolScratch[float64](&convColPoolF64, colHeight*colWidth)
	colBuf := *colp
	defer convColPoolF64.Put(colp)
	im2colFloat64(colBuf, inputData, dims)

	// MatMul into pooled scratch (len == len(outputData)); matMulColBufFloat64
	// writes every element, so the un-zeroed buffer is safe.
	matp := poolScratch[float64](&convOutPoolF64, COut*colHeight)
	matOut := *matp
	defer convOutPoolF64.Put(matp)
	matMulColBufFloat64(matOut, kernelData, colBuf, COut, colHeight, colWidth)

	// Rearrange [C_out, N*H_out*W_out] -> [N, C_out, H_out, W_out], permuting the
	// matmul scratch straight into the output (no intermediate copy).
	rearrangeOutputFloat64(outputData, matOut, N, COut, HOut, WOut, colHeight)
}

// im2colFloat64Stride1NoPad is optimized for stride=1, padding=0.
// Compiler can better optimize with hardcoded stride=1 (no bounds checks for padding).
func im2colFloat64Stride1NoPad(colBuf, inputData []float64, dims *ConvDims) {
	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut

	colWidth := CIn * KH * KW
	colIdx := 0

	for n := 0; n < N; n++ {
		batchOffset := n * CIn * H * W
		batchData := inputData[batchOffset : batchOffset+CIn*H*W]

		for outH := 0; outH < HOut; outH++ {
			for outW := 0; outW < WOut; outW++ {
				// With stride=1, padding=0: hStart = outH, wStart = outW
				rowOffset := colIdx * colWidth
				rowData := colBuf[rowOffset : rowOffset+colWidth]

				bufIdx := 0
				for c := 0; c < CIn; c++ {
					channelOffset := c * H * W
					channelData := batchData[channelOffset : channelOffset+H*W]

					for kh := 0; kh < KH; kh++ {
						h := outH + kh // stride=1: no multiplication
						for kw := 0; kw < KW; kw++ {
							w := outW + kw // stride=1: no multiplication
							// No padding check needed (padding=0 guaranteed)
							rowData[bufIdx] = channelData[h*W+w]
							bufIdx++
						}
					}
				}

				colIdx++
			}
		}
	}
}

func im2colFloat64(colBuf, inputData []float64, dims *ConvDims) {
	N := dims.N
	CIn := dims.CIn
	H := dims.H
	W := dims.W
	KH := dims.KH
	KW := dims.KW
	HOut := dims.HOut
	WOut := dims.WOut
	stride := dims.Stride
	padding := dims.Padding

	colWidth := CIn * KH * KW
	colIdx := 0

	for n := 0; n < N; n++ {
		// Pre-slice batch: eliminates n*CIn*H*W bounds check
		batchOffset := n * CIn * H * W
		batchData := inputData[batchOffset : batchOffset+CIn*H*W]

		for outH := 0; outH < HOut; outH++ {
			for outW := 0; outW < WOut; outW++ {
				hStart := outH*stride - padding
				wStart := outW*stride - padding

				// Pre-slice output row: eliminates colIdx*colWidth bounds check
				rowOffset := colIdx * colWidth
				rowData := colBuf[rowOffset : rowOffset+colWidth]

				bufIdx := 0
				for c := 0; c < CIn; c++ {
					// Pre-slice channel: eliminates c*H*W bounds check
					channelOffset := c * H * W
					channelData := batchData[channelOffset : channelOffset+H*W]

					for kh := 0; kh < KH; kh++ {
						h := hStart + kh
						for kw := 0; kw < KW; kw++ {
							w := wStart + kw

							if h >= 0 && h < H && w >= 0 && w < W {
								// Valid input position: single bounds check via pre-slice
								rowData[bufIdx] = channelData[h*W+w]
							} else {
								rowData[bufIdx] = 0.0
							}
							bufIdx++
						}
					}
				}
				colIdx++
			}
		}
	}
}
