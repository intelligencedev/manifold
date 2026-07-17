package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// MaxPool2D is a 2D max pooling layer.
//
// Max pooling reduces spatial dimensions by taking the maximum value
// in each non-overlapping window. Unlike Conv2D, MaxPool2D has no
// learnable parameters.
//
// Input shape:  [batch, channels, height, width]
// Output shape: [batch, channels, out_height, out_width]
//
// Where:
//
//	out_height = (height - kernelSize) / stride + 1
//	out_width = (width - kernelSize) / stride + 1
//
// Common configurations:
//   - 2x2 pool, stride=2: Reduces spatial dimensions by half (most common)
//   - 3x3 pool, stride=2: Aggressive downsampling
//   - 2x2 pool, stride=1: Overlapping pooling (less common)
//
// Example:
//
//	// Create 2x2 max pooling with stride 2
//	pool := nn.NewMaxPool2D(2, 2, backend)
//
//	// Forward pass
//	input := tensor.Randn[float32](tensor.Shape{32, 64, 28, 28}, backend)
//	output := pool.Forward(input) // [32, 64, 14, 14]
type MaxPool2D[B tensor.Backend] struct {
	kernelSize int
	stride     int
	backend    B
}

// NewMaxPool2D creates a new 2D max pooling layer.
//
// Parameters:
//   - kernelSize: Size of pooling window (square)
//   - stride: Stride for pooling (typically same as kernelSize for non-overlapping)
//   - backend: Backend for computation
//
// Common patterns:
//   - NewMaxPool2D(2, 2, backend): Standard 2x2 non-overlapping pooling
//   - NewMaxPool2D(3, 2, backend): Overlapping 3x3 pooling with stride 2
func NewMaxPool2D[B tensor.Backend](kernelSize, stride int, backend B) *MaxPool2D[B] {
	if kernelSize <= 0 {
		panic(fmt.Sprintf("maxpool2d: invalid kernel size %d", kernelSize))
	}
	if stride <= 0 {
		panic(fmt.Sprintf("maxpool2d: invalid stride %d", stride))
	}

	return &MaxPool2D[B]{
		kernelSize: kernelSize,
		stride:     stride,
		backend:    backend,
	}
}

// Forward performs the forward pass.
//
// Input: [batch, channels, height, width]
// Output: [batch, channels, out_height, out_width].
func (m *MaxPool2D[B]) Forward(input *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// Validate input shape
	inputShape := input.Shape()
	if len(inputShape) != 4 {
		panic(fmt.Sprintf("maxpool2d: expected 4D input [N,C,H,W], got %dD", len(inputShape)))
	}

	// Perform max pooling
	outputRaw := m.backend.MaxPool2D(input.Raw(), m.kernelSize, m.stride)

	// Wrap in Tensor for high-level API
	return tensor.New[float32, B](outputRaw, m.backend)
}

// Parameters returns all trainable parameters (empty for MaxPool2D).
//
// MaxPool2D has no learnable parameters, so this always returns an empty slice.
func (m *MaxPool2D[B]) Parameters() []*Parameter[B] {
	return []*Parameter[B]{}
}

// String returns a string representation of the layer.
func (m *MaxPool2D[B]) String() string {
	return fmt.Sprintf("MaxPool2D(kernel_size=%d, stride=%d)",
		m.kernelSize, m.stride)
}

// KernelSize returns the pooling kernel size.
func (m *MaxPool2D[B]) KernelSize() int {
	return m.kernelSize
}

// Stride returns the stride.
func (m *MaxPool2D[B]) Stride() int {
	return m.stride
}

// ComputeOutputSize computes output spatial dimensions for given input size.
//
// Returns: [out_height, out_width].
func (m *MaxPool2D[B]) ComputeOutputSize(inputH, inputW int) [2]int {
	outH := (inputH-m.kernelSize)/m.stride + 1
	outW := (inputW-m.kernelSize)/m.stride + 1
	return [2]int{outH, outW}
}
