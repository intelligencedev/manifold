// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

// Package tensor provides type-safe tensor operations for the Born ML framework.
//
// # Overview
//
// Tensors are the fundamental data structure in Born. This package provides:
//   - Generic type-safe tensors (Tensor[T, B])
//   - NumPy-style broadcasting
//   - Zero-copy operations where possible
//   - Device abstraction (CPU, CUDA)
//
// # Basic Usage
//
//	import (
//	    "github.com/born-ml/born/tensor"
//	    "github.com/born-ml/born/backend/cpu"
//	)
//
//	func main() {
//	    backend := cpu.New()
//
//	    // Create tensors
//	    x := tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
//	    y := tensor.Ones[float32](tensor.Shape{2, 3}, backend)
//
//	    // Tensor operations
//	    z := x.Add(y)
//	    result := x.MatMul(y.Transpose())
//	}
//
// # Supported Data Types
//
// The tensor package supports the following data types via the DType constraint:
//   - float32, float64 (floating-point)
//   - int32, int64 (signed integers)
//   - uint8 (unsigned integers, useful for images)
//   - bool (boolean masks)
//
// # Device Support
//
// Tensors can reside on different devices:
//   - CPU: Pure Go implementation (v0.1.0+)
//   - WebGPU: Zero-CGO GPU acceleration (v0.2.0+, Windows)
//   - CUDA: GPU support (planned for v0.5.0)
//
// # Broadcasting
//
// Tensor operations follow NumPy broadcasting rules:
//
//	a := tensor.Zeros[float32](tensor.Shape{3, 1}, backend)     // (3, 1)
//	b := tensor.Ones[float32](tensor.Shape{3, 4}, backend)      // (3, 4)
//	c := a.Add(b)                                                // (3, 4)
//
// # Memory Management
//
// Tensors use zero-copy operations where possible. The underlying data is
// reference-counted and automatically freed when no longer needed.
//
// # Available Operations (v0.3.0+)
//
// Tensor[T, B] provides 31 type-safe operations:
//
// Scalar operations:
//
//	y := x.MulScalar(2.0)    // Multiply by scalar
//	y := x.AddScalar(1.0)    // Add scalar
//	y := x.SubScalar(0.5)    // Subtract scalar
//	y := x.DivScalar(2.0)    // Divide by scalar
//
// Math operations:
//
//	y := x.Exp()             // Exponential
//	y := x.Log()             // Natural logarithm
//	y := x.Sqrt()            // Square root
//	y := x.Rsqrt()           // Reciprocal square root
//	y := x.Cos()             // Cosine
//	y := x.Sin()             // Sine
//
// Comparison operations (return Tensor[bool, B]):
//
//	mask := x.Greater(y)     // or x.Gt(y)
//	mask := x.Lower(y)       // or x.Lt(y)
//	mask := x.Equal(y)       // or x.Eq(y)
//
// Type conversion:
//
//	i := x.Int32()           // Convert to int32
//	f := x.Float64()         // Convert to float64
//
// See method documentation for full list of operations.
package tensor
