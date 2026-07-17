package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// Conv2DOp records a 2D convolution operation for autodiff.
//
// Forward: output = Conv2D(input, kernel, stride, padding)
//
// Backward (gradients):
//   - d_input:  "transposed convolution" or "deconvolution" of d_output with kernel
//   - d_kernel: convolution of input with d_output
//
// References:
//   - "A guide to convolution arithmetic for deep learning" (Dumoulin & Visin, 2016)
//   - CS231n: Convolutional Neural Networks for Visual Recognition
type Conv2DOp struct {
	input   *tensor.RawTensor
	kernel  *tensor.RawTensor
	output  *tensor.RawTensor
	stride  int
	padding int
}

// NewConv2DOp creates a new Conv2D operation.
func NewConv2DOp(input, kernel, output *tensor.RawTensor, stride, padding int) *Conv2DOp {
	return &Conv2DOp{
		input:   input,
		kernel:  kernel,
		output:  output,
		stride:  stride,
		padding: padding,
	}
}

// Inputs returns the input tensors.
func (op *Conv2DOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input, op.kernel}
}

// Output returns the output tensor.
func (op *Conv2DOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes gradients for Conv2D.
//
// This is pure orchestration - delegates computation to backend.
//
// Given:
//   - outputGrad: ∂L/∂output [N, C_out, H_out, W_out]
//
// Compute:
//   - inputGrad:  ∂L/∂input  [N, C_in, H, W]
//   - kernelGrad: ∂L/∂kernel [C_out, C_in, K_h, K_w]
//
// References:
//   - Burn framework: crates/burn-autodiff/src/ops/module.rs (conv2d backward)
func (op *Conv2DOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Orchestration: delegate to backend
	inputGrad := backend.Conv2DInputBackward(op.input, op.kernel, outputGrad, op.stride, op.padding)
	kernelGrad := backend.Conv2DKernelBackward(op.input, op.kernel, outputGrad, op.stride, op.padding)

	return []*tensor.RawTensor{inputGrad, kernelGrad}
}
