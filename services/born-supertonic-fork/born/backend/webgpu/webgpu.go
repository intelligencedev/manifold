//go:build windows || linux

// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

// Package webgpu provides the WebGPU backend for GPU-accelerated tensor operations.
//
// WebGPU is a cross-platform graphics and compute API that works on:
//   - Windows (via Dawn/D3D12)
//   - macOS (via Dawn/Metal)
//   - Linux (via Dawn/Vulkan)
//   - Web browsers (via wasm)
//
// Example:
//
//	import (
//	    "github.com/born-ml/born/autodiff"
//	    "github.com/born-ml/born/backend/webgpu"
//	    "github.com/born-ml/born/tensor"
//	)
//
//	func main() {
//	    gpu, err := webgpu.New()
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    defer gpu.Release()
//
//	    backend := autodiff.New(gpu)
//	    x := tensor.Randn[float32](tensor.Shape{1024, 1024}, backend)
//	}
package webgpu

import (
	internalwebgpu "github.com/born-ml/born/internal/backend/webgpu"
	"github.com/born-ml/born/tensor"
)

// Backend represents the WebGPU backend implementation for GPU-accelerated
// tensor operations.
type Backend = internalwebgpu.Backend

// Compile-time check that Backend implements tensor.Backend.
var _ tensor.Backend = (*Backend)(nil)

// New creates a new WebGPU backend.
//
// This function initializes the WebGPU device and returns a backend
// ready for tensor operations. Call Release() when done to free GPU resources.
//
// Returns an error if WebGPU initialization fails (e.g., no compatible GPU).
func New() (*Backend, error) {
	return internalwebgpu.New()
}

// IsAvailable checks if WebGPU is available on the current system.
//
// This function attempts to initialize a WebGPU adapter to verify
// that a compatible GPU and drivers are present. It's useful for
// graceful fallback to CPU backend when GPU is not available.
//
// Example:
//
//	if webgpu.IsAvailable() {
//	    gpu, _ := webgpu.New()
//	    backend = autodiff.New(gpu)
//	} else {
//	    backend = autodiff.New(cpu.New())
//	}
func IsAvailable() bool {
	return internalwebgpu.IsAvailable()
}
