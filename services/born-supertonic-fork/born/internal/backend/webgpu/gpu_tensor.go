//go:build windows || linux

// Package webgpu implements the WebGPU backend for GPU-accelerated tensor operations.
package webgpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
	wgpu "github.com/gogpu/wgpu"
)

// GPUTensor holds tensor data in GPU memory without transferring to CPU.
// This enables efficient GPU-to-GPU operations without the overhead of readBuffer() calls.
type GPUTensor struct {
	buffer       *wgpu.Buffer    // GPU-resident data buffer
	shape        tensor.Shape    // Tensor dimensions
	dtype        tensor.DataType // Data type (Float32, Int32, etc.)
	strides      []int           // Memory strides for layout
	backend      *Backend        // Reference to WebGPU backend
	computed     bool            // false = lazy (not computed), true = computed
	bufferSize   uint64          // Actual buffer size (aligned to 4 bytes for WebGPU)
	dependencies []*GPUTensor    // Lazy evaluation dependency graph
	computeFunc  func()          // Function to compute this tensor (for lazy eval)
	requiresGrad bool            // Whether to compute gradients for this tensor
	grad         *GPUTensor      // Accumulated gradient (also on GPU!)
	tape         *GPUTape        // Gradient tape for recording operations
}

// Eval forces computation of lazy tensor using batched submission.
// Collects all dependencies and submits them in a single GPU command buffer.
// This reduces GPU overhead compared to submitting each operation separately.
func (t *GPUTensor) Eval() *GPUTensor {
	if t.computed {
		return t
	}

	// Collect all uncomputed dependencies in topological order
	deps := t.collectDependencies()

	// If no dependencies or already computed, nothing to do
	if len(deps) == 0 {
		t.computed = true
		return t
	}

	// Create batch for single submission
	batch := t.backend.NewBatch()

	// Add all dependency computations to the batch
	hasOps := false
	for _, dep := range deps {
		if dep.computeFunc != nil {
			batch.Add("compute", dep, dep.computeFunc)
			hasOps = true
		}
	}

	// Submit batch only if there are operations to execute
	if hasOps {
		batch.Submit()
	}

	// Mark all dependencies as computed (including those without computeFunc)
	for _, dep := range deps {
		dep.computed = true
	}

	return t
}

// collectDependencies performs topological sort of the computation graph.
// Returns tensors in execution order (dependencies before dependents).
func (t *GPUTensor) collectDependencies() []*GPUTensor {
	visited := make(map[*GPUTensor]bool)
	result := make([]*GPUTensor, 0)

	var visit func(*GPUTensor)
	visit = func(tensor *GPUTensor) {
		if visited[tensor] || tensor.computed {
			return
		}
		visited[tensor] = true

		// Visit dependencies first (depth-first)
		for _, dep := range tensor.dependencies {
			visit(dep)
		}

		// Add this tensor after its dependencies
		result = append(result, tensor)
	}

	visit(t)
	return result
}

// ToCPU transfers tensor data from GPU to CPU memory.
// This is an expensive operation and should be used sparingly.
// Returns a new RawTensor with data copied from GPU.
func (t *GPUTensor) ToCPU() *tensor.RawTensor {
	// Calculate actual data size (without padding)
	numElements := t.shape.NumElements()
	actualByteSize := numElements * t.dtype.Size()

	// Read data from GPU buffer using stored buffer size (aligned)
	data, err := t.backend.readBuffer(t.buffer, t.bufferSize)
	if err != nil {
		panic(fmt.Sprintf("webgpu: ToCPU failed: %v", err))
	}

	// Create RawTensor with CPU data
	raw, err := tensor.NewRaw(t.shape, t.dtype, tensor.CPU)
	if err != nil {
		panic(fmt.Sprintf("webgpu: ToCPU: failed to create RawTensor: %v", err))
	}

	// Copy only the actual data (not padding)
	copy(raw.Data(), data[:actualByteSize])

	return raw
}

// Item returns the single scalar value from a tensor.
// This is useful for extracting loss values during training.
// Panics if tensor has more than one element.
func (t *GPUTensor) Item() float32 {
	if t.shape.NumElements() != 1 {
		panic(fmt.Sprintf("webgpu: Item() requires scalar tensor, got shape %v", t.shape))
	}

	// Transfer to CPU and extract value
	raw := t.ToCPU()

	// Extract based on dtype
	switch t.dtype {
	case tensor.Float32:
		return raw.AsFloat32()[0]
	case tensor.Float64:
		return float32(raw.AsFloat64()[0])
	case tensor.Int32:
		return float32(raw.AsInt32()[0])
	case tensor.Int64:
		return float32(raw.AsInt64()[0])
	case tensor.Uint8:
		return float32(raw.AsUint8()[0])
	default:
		panic(fmt.Sprintf("webgpu: Item() unsupported dtype: %v", t.dtype))
	}
}

// Shape returns the tensor's shape.
func (t *GPUTensor) Shape() tensor.Shape {
	return t.shape
}

// DType returns the tensor's data type.
func (t *GPUTensor) DType() tensor.DataType {
	return t.dtype
}

// NumElements returns the total number of elements in the tensor.
func (t *GPUTensor) NumElements() int {
	return t.shape.NumElements()
}

// ByteSize returns the total memory size in bytes.
func (t *GPUTensor) ByteSize() uint64 {
	return uint64(t.NumElements() * t.dtype.Size()) //nolint:gosec // G115: integer overflow conversion int -> uint64
}

// Buffer returns the underlying GPU buffer.
// This is exposed for internal backend operations.
func (t *GPUTensor) Buffer() *wgpu.Buffer {
	return t.buffer
}

// Release releases the GPU buffer and frees memory.
// This should be called when the tensor is no longer needed.
func (t *GPUTensor) Release() {
	if t.buffer != nil {
		t.buffer.Release()
		t.buffer = nil
	}
}

// SetRequiresGrad sets whether this tensor requires gradient computation.
// Returns the tensor for method chaining.
// Note: PyTorch uses requires_grad_ (underscore suffix for in-place).
// In Go, we use SetRequiresGrad for clarity.
func (t *GPUTensor) SetRequiresGrad(requires bool) *GPUTensor {
	t.requiresGrad = requires
	return t
}

// RequiresGrad returns whether this tensor requires gradient computation.
func (t *GPUTensor) RequiresGrad() bool {
	return t.requiresGrad
}

// Grad returns the accumulated gradient for this tensor.
// Returns nil if no gradient has been computed.
func (t *GPUTensor) Grad() *GPUTensor {
	return t.grad
}

// ZeroGrad clears the accumulated gradient.
func (t *GPUTensor) ZeroGrad() {
	if t.grad != nil {
		t.grad.Release()
		t.grad = nil
	}
}

// Backward computes gradients for this tensor.
// This is a convenience method that creates a gradient of ones and calls tape.Backward().
func (t *GPUTensor) Backward() {
	if t.tape == nil {
		panic("webgpu: Backward() called on tensor without tape")
	}

	// Create gradient of ones with same shape as this tensor
	onesRaw, err := tensor.NewRaw(t.shape, t.dtype, tensor.CPU)
	if err != nil {
		panic(fmt.Sprintf("webgpu: Backward: failed to create ones gradient: %v", err))
	}

	// Fill with ones
	switch t.dtype {
	case tensor.Float32:
		data := onesRaw.AsFloat32()
		for i := range data {
			data[i] = 1.0
		}
	case tensor.Float64:
		data := onesRaw.AsFloat64()
		for i := range data {
			data[i] = 1.0
		}
	case tensor.Int32:
		data := onesRaw.AsInt32()
		for i := range data {
			data[i] = 1
		}
	default:
		panic(fmt.Sprintf("webgpu: Backward: unsupported dtype: %v", t.dtype))
	}

	// Upload to GPU
	onesGPU := t.backend.UploadTensor(onesRaw)
	defer onesGPU.Release()

	// Compute gradients
	grads := t.tape.Backward(onesGPU)

	// Store gradients in tensors
	for tensor, grad := range grads {
		if tensor.requiresGrad {
			if tensor.grad == nil {
				tensor.grad = grad
			} else {
				// Accumulate gradient
				oldGrad := tensor.grad
				tensor.grad = t.backend.AddGPU(oldGrad, grad)
				oldGrad.Release()
			}
		}
	}
}
