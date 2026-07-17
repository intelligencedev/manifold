// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package cpu

import (
	internalcpu "github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/tensor"
)

// Backend represents the CPU backend implementation.
//
// CPU backend provides pure Go implementations of all tensor operations
// with SIMD optimizations where available.
type Backend = internalcpu.CPUBackend

// Compile-time check that Backend implements tensor.Backend.
var _ tensor.Backend = (*Backend)(nil)

// New creates a new CPU backend.
//
// Example:
//
//	import (
//	    "github.com/born-ml/born/backend/cpu"
//	    "github.com/born-ml/born/tensor"
//	)
//
//	func main() {
//	    backend := cpu.New()
//	    x := tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
//	}
func New() *Backend {
	return internalcpu.New()
}
