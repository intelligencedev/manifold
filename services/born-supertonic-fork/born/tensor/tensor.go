// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

// Package tensor provides the public API for tensor operations in the Born ML framework.
//
// The package defines core interfaces and types for type-safe tensor operations:
//   - Tensor[T, B]: High-level generic tensor with type safety
//   - RawTensor: Low-level tensor interface for advanced use cases
//   - Backend: Interface for device-specific compute implementations
//   - Shape, DataType, Device: Core type definitions
//
// Example:
//
//	backend := cpu.New()
//	x := tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
//	y := tensor.Ones[float32](tensor.Shape{2, 3}, backend)
//	z := x.Add(y)  // Element-wise addition
package tensor

import (
	"github.com/born-ml/born/internal/tensor"
)

// Type aliases for public API

// DType is a constraint for tensor data types.
// Supported types: float32, float64, int32, int64, uint8, bool.
type DType = tensor.DType

// DataType represents the underlying data type of a tensor.
type DataType = tensor.DataType

// Data type constants.
const (
	Float32 DataType = tensor.Float32
	Float64 DataType = tensor.Float64
	Int32   DataType = tensor.Int32
	Int64   DataType = tensor.Int64
	Uint8   DataType = tensor.Uint8
	Bool    DataType = tensor.Bool
)

// Device represents the device where tensor data resides.
type Device = tensor.Device

// Device constants.
const (
	CPU    Device = tensor.CPU
	CUDA   Device = tensor.CUDA
	Vulkan Device = tensor.Vulkan
	Metal  Device = tensor.Metal
	WebGPU Device = tensor.WebGPU
)

// Shape represents the dimensions of a tensor.
// Example: Shape{2, 3, 4} represents a 3D tensor with dimensions 2×3×4.
type Shape = tensor.Shape

// Backend is defined in backend.go as a proper interface.

// Tensor is a generic type-safe tensor.
//
// T is the data type (float32, float64, int32, int64, uint8, bool).
// B is the backend implementation (CPU, WebGPU, etc.).
//
// Tensor provides a high-level API for tensor operations with:
//   - Type safety via Go generics
//   - Automatic differentiation support (via autodiff.Backend)
//   - Multiple backend support (CPU, GPU)
//   - Efficient memory management with copy-on-write
//
// Example:
//
//	backend := cpu.New()
//	x := tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
//	y := tensor.Ones[float32](tensor.Shape{2, 3}, backend)
//	z := x.Add(y)  // Element-wise addition
type Tensor[T DType, B Backend] = tensor.Tensor[T, B]

// Creation functions

// Zeros creates a tensor filled with zeros.
//
// Example:
//
//	backend := cpu.New()
//	x := tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
func Zeros[T DType, B Backend](shape Shape, b B) *Tensor[T, B] {
	return tensor.Zeros[T, B](shape, b)
}

// Ones creates a tensor filled with ones.
//
// Example:
//
//	backend := cpu.New()
//	x := tensor.Ones[float32](tensor.Shape{2, 3}, backend)
func Ones[T DType, B Backend](shape Shape, b B) *Tensor[T, B] {
	return tensor.Ones[T, B](shape, b)
}

// Full creates a tensor filled with a specific value.
//
// Example:
//
//	backend := cpu.New()
//	x := tensor.Full[float32](tensor.Shape{2, 3}, 3.14, backend)
func Full[T DType, B Backend](shape Shape, value T, b B) *Tensor[T, B] {
	return tensor.Full[T, B](shape, value, b)
}

// Randn creates a tensor filled with random values from standard normal distribution N(0, 1).
//
// Example:
//
//	backend := cpu.New()
//	x := tensor.Randn[float32](tensor.Shape{2, 3}, backend)
func Randn[T DType, B Backend](shape Shape, b B) *Tensor[T, B] {
	return tensor.Randn[T, B](shape, b)
}

// Rand creates a tensor filled with random values from uniform distribution U(0, 1).
//
// Example:
//
//	backend := cpu.New()
//	x := tensor.Rand[float32](tensor.Shape{2, 3}, backend)
func Rand[T DType, B Backend](shape Shape, b B) *Tensor[T, B] {
	return tensor.Rand[T, B](shape, b)
}

// Arange creates a 1D tensor with values from start to end (exclusive).
//
// Example:
//
//	backend := cpu.New()
//	x := tensor.Arange[float32](0, 10, backend)  // [0, 1, 2, ..., 9]
func Arange[T DType, B Backend](start, end T, b B) *Tensor[T, B] {
	return tensor.Arange[T, B](start, end, b)
}

// Eye creates a 2D identity matrix.
//
// Example:
//
//	backend := cpu.New()
//	identity := tensor.Eye[float32](3, backend)  // 3x3 identity matrix
func Eye[T DType, B Backend](n int, b B) *Tensor[T, B] {
	return tensor.Eye[T, B](n, b)
}

// FromSlice creates a tensor from a Go slice.
//
// Example:
//
//	backend := cpu.New()
//	data := []float32{1, 2, 3, 4, 5, 6}
//	x, err := tensor.FromSlice(data, tensor.Shape{2, 3}, backend)
func FromSlice[T DType, B Backend](data []T, shape Shape, b B) (*Tensor[T, B], error) {
	return tensor.FromSlice[T, B](data, shape, b)
}

// New creates a tensor from a raw tensor.
//
// This is a low-level function. Most users should use creation functions like
// Zeros, Ones, or FromSlice instead.
func New[T DType, B Backend](raw *RawTensor, b B) *Tensor[T, B] {
	return tensor.New[T, B](raw, b)
}

// NewRaw creates a new raw tensor with the given shape, dtype, and device.
//
// This is a low-level function. Most users should use high-level creation functions instead.
func NewRaw(shape Shape, dtype DataType, device Device) (*RawTensor, error) {
	return tensor.NewRaw(shape, dtype, device)
}

// Manipulation functions

// Cat concatenates tensors along a dimension.
//
// Example:
//
//	backend := cpu.New()
//	a := tensor.Ones[float32](tensor.Shape{2, 3}, backend)
//	b := tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
//	c := tensor.Cat([]*tensor.Tensor[float32, B]{a, b}, 0)  // Shape: [4, 3]
func Cat[T DType, B Backend](tensors []*Tensor[T, B], dim int) *Tensor[T, B] {
	return tensor.Cat(tensors, dim)
}

// Where selects elements from x or y based on condition.
//
// Example:
//
//	backend := cpu.New()
//	cond := tensor.Full[bool](tensor.Shape{3}, true, backend)
//	x := tensor.Full[float32](tensor.Shape{3}, 1.0, backend)
//	y := tensor.Full[float32](tensor.Shape{3}, 0.0, backend)
//	result := tensor.Where(cond, x, y)  // [1.0, 1.0, 1.0]
func Where[T DType, B Backend](cond *Tensor[bool, B], x, y *Tensor[T, B]) *Tensor[T, B] {
	return tensor.Where(cond, x, y)
}

// Utility functions

// BroadcastShapes computes the broadcast shape for two shapes following NumPy broadcasting rules.
// Returns the resulting shape and two flags indicating if each operand needs broadcasting.
//
// Example:
//
//	resultShape, needsBroadcastA, err := tensor.BroadcastShapes(
//	    tensor.Shape{3, 1},
//	    tensor.Shape{3, 4},
//	)
//	// resultShape = [3, 4], needsBroadcastA = true
func BroadcastShapes(a, b Shape) (Shape, bool, error) {
	return tensor.BroadcastShapes(a, b)
}

// BroadcastShapesMatMul computes the broadcast shape for two shapes specifically for matrix multiplication following NumPy broadcasting rules.
// Returns the resulting shape, a flag indicating if broadcasting is needed among batch dimensions, and an error if incompatible.
//
// Example:
//
//	resultShape, needsBroadcast, err := tensor.BroadcastShapesMatMul(
//	    tensor.Shape{2, 3, 4},
//	    tensor.Shape{2, 4, 5},
//	)
//	// resultShape = [2, 3, 5], needsBroadcast = false
func BroadcastShapesMatMul(a, b Shape) (Shape, bool, error) {
	return tensor.BroadcastShapesMatMul(a, b)
}
