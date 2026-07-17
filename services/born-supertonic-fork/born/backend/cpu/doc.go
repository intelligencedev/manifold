// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

// Package cpu provides a pure Go CPU backend for tensor operations.
//
// # Overview
//
// This package implements a CPU backend with:
//   - Pure Go implementation (no CGO)
//   - Im2col algorithm for efficient convolutions
//   - Float32 and Float64 support
//   - Batch processing
//   - NumPy-compatible broadcasting
//
// # Basic Usage
//
//	import (
//	    "github.com/born-ml/born/backend/cpu"
//	    "github.com/born-ml/born/tensor"
//	    "github.com/born-ml/born/nn"
//	)
//
//	func main() {
//	    // Create CPU backend
//	    backend := cpu.New()
//
//	    // Use with tensors
//	    x := tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
//	    y := tensor.Ones[float32](tensor.Shape{2, 3}, backend)
//	    z := x.Add(y)
//
//	    // Use with neural networks
//	    model := nn.NewLinear(784, 10, backend)
//	}
//
// # Performance
//
// The CPU backend is optimized for training on CPUs:
//   - Efficient matrix multiplication
//   - Im2col-based convolutions
//   - SIMD optimizations (where available)
//
// For GPU acceleration, see the cuda package (planned for v0.3.0).
//
// # Thread Safety
//
// The CPU backend is safe for concurrent use. Each tensor operation
// is isolated and does not share mutable state.
package cpu
