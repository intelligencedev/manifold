package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Conv2D is a 2D convolutional layer.
//
// Performs convolution: output = Conv2D(input, weight) + bias
//
// Input shape:  [batch, in_channels, height, width]
// Weight shape: [out_channels, in_channels, kernel_h, kernel_w]
// Bias shape:   [out_channels]
// Output shape: [batch, out_channels, out_h, out_w]
//
// Where:
//
//	out_h = (height + 2*padding - kernel_h) / stride + 1
//	out_w = (width + 2*padding - kernel_w) / stride + 1
//
// Example:
//
//	// Create 2D conv: 1 channel -> 6 channels, 5x5 kernel
//	conv := nn.NewConv2D(1, 6, 5, 5, 1, 0, true, backend)
//
//	// Forward pass
//	input := tensor.Zeros[float32](tensor.Shape{32, 1, 28, 28}, backend) // MNIST-like
//	output := conv.Forward(input) // [32, 6, 24, 24]
type Conv2D[B tensor.Backend] struct {
	inChannels  int
	outChannels int
	kernelSize  [2]int
	stride      int
	padding     int
	useBias     bool

	weight *Parameter[B] // [out_channels, in_channels, kernel_h, kernel_w]
	bias   *Parameter[B] // [out_channels] or nil

	backend B
}

// NewConv2D creates a new 2D convolutional layer with Xavier initialization.
//
// Parameters:
//   - inChannels: Number of input channels
//   - outChannels: Number of output channels (number of filters)
//   - kernelH, kernelW: Kernel dimensions
//   - stride: Stride for convolution (commonly 1 or 2)
//   - padding: Zero padding to apply to input (commonly 0, 1, 2)
//   - useBias: Whether to include bias term
//   - backend: Backend for computation
//
// Initialization:
//   - Weights: Xavier/Glorot uniform initialization
//   - Bias: Zeros
func NewConv2D[B tensor.Backend](
	inChannels, outChannels int,
	kernelH, kernelW int,
	stride, padding int,
	useBias bool,
	backend B,
) *Conv2D[B] {
	if inChannels <= 0 || outChannels <= 0 {
		panic(fmt.Sprintf("conv2d: invalid channels in=%d, out=%d", inChannels, outChannels))
	}
	if kernelH <= 0 || kernelW <= 0 {
		panic(fmt.Sprintf("conv2d: invalid kernel size h=%d, w=%d", kernelH, kernelW))
	}
	if stride <= 0 {
		panic(fmt.Sprintf("conv2d: invalid stride %d", stride))
	}
	if padding < 0 {
		panic(fmt.Sprintf("conv2d: invalid padding %d", padding))
	}

	// Create weight parameter [out_channels, in_channels, kernel_h, kernel_w]
	weightShape := tensor.Shape{outChannels, inChannels, kernelH, kernelW}

	// Xavier initialization for weights
	// For Conv2D:
	//   fan_in = in_channels * kernel_h * kernel_w
	//   fan_out = out_channels * kernel_h * kernel_w
	fanIn := inChannels * kernelH * kernelW
	fanOut := outChannels * kernelH * kernelW
	weight := Xavier(fanIn, fanOut, weightShape, backend)

	weightParam := NewParameter("conv2d.weight", weight)

	// Create bias parameter if needed
	var biasParam *Parameter[B]
	if useBias {
		bias := Zeros(tensor.Shape{outChannels}, backend)
		biasParam = NewParameter("conv2d.bias", bias)
	}

	return &Conv2D[B]{
		inChannels:  inChannels,
		outChannels: outChannels,
		kernelSize:  [2]int{kernelH, kernelW},
		stride:      stride,
		padding:     padding,
		useBias:     useBias,
		weight:      weightParam,
		bias:        biasParam,
		backend:     backend,
	}
}

// Forward performs the forward pass.
//
// Input: [batch, in_channels, height, width]
// Output: [batch, out_channels, out_h, out_w].
func (c *Conv2D[B]) Forward(input *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// Validate input shape
	inputShape := input.Shape()
	if len(inputShape) != 4 {
		panic(fmt.Sprintf("conv2d: expected 4D input [N,C,H,W], got %dD", len(inputShape)))
	}
	if inputShape[1] != c.inChannels {
		panic(fmt.Sprintf("conv2d: input channels %d != expected %d", inputShape[1], c.inChannels))
	}

	// Perform convolution
	outputRaw := c.backend.Conv2D(
		input.Raw(),
		c.weight.Tensor().Raw(),
		c.stride,
		c.padding,
	)

	// Wrap in Tensor for high-level API
	output := tensor.New[float32, B](outputRaw, c.backend)

	// Add bias if present
	if c.useBias {
		// Bias shape: [out_channels]
		// Output shape: [batch, out_channels, out_h, out_w]
		// Need to reshape bias to [1, out_channels, 1, 1] for broadcasting

		// Reshape bias using Tensor API (handles autodiff properly)
		biasReshaped := c.bias.Tensor().Reshape(1, c.outChannels, 1, 1)

		// Add bias using Tensor API (broadcasts and records on tape)
		output = output.Add(biasReshaped)
	}

	return output
}

// Parameters returns all trainable parameters.
func (c *Conv2D[B]) Parameters() []*Parameter[B] {
	if c.useBias {
		return []*Parameter[B]{c.weight, c.bias}
	}
	return []*Parameter[B]{c.weight}
}

// String returns a string representation of the layer.
func (c *Conv2D[B]) String() string {
	return fmt.Sprintf("Conv2D(in_channels=%d, out_channels=%d, kernel_size=(%d, %d), stride=%d, padding=%d, bias=%v)",
		c.inChannels, c.outChannels,
		c.kernelSize[0], c.kernelSize[1],
		c.stride, c.padding, c.useBias)
}

// OutChannels returns the number of output channels.
func (c *Conv2D[B]) OutChannels() int {
	return c.outChannels
}

// InChannels returns the number of input channels.
func (c *Conv2D[B]) InChannels() int {
	return c.inChannels
}

// KernelSize returns the kernel size [height, width].
func (c *Conv2D[B]) KernelSize() [2]int {
	return c.kernelSize
}

// Stride returns the stride.
func (c *Conv2D[B]) Stride() int {
	return c.stride
}

// Padding returns the padding.
func (c *Conv2D[B]) Padding() int {
	return c.padding
}

// ComputeOutputSize computes output spatial dimensions for given input size.
//
// Returns: [out_height, out_width].
func (c *Conv2D[B]) ComputeOutputSize(inputH, inputW int) [2]int {
	outH := (inputH+2*c.padding-c.kernelSize[0])/c.stride + 1
	outW := (inputW+2*c.padding-c.kernelSize[1])/c.stride + 1
	return [2]int{outH, outW}
}
