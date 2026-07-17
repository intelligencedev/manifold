//go:build windows || linux

package webgpu

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
	"github.com/gogpu/gputypes"
	wgpu "github.com/gogpu/wgpu"
)

// GPUTape records GPU operations for backward pass.
// All operations and gradients stay on GPU for maximum performance.
type GPUTape struct {
	backend    *Backend
	operations []gpuOperation
	enabled    bool
}

// gpuOperation represents a recorded GPU operation in the computation graph.
type gpuOperation struct {
	name     string
	inputs   []*GPUTensor
	output   *GPUTensor
	backward func(grad *GPUTensor) []*GPUTensor
}

// NewGPUTape creates a new gradient tape for GPU operations.
func NewGPUTape(b *Backend) *GPUTape {
	return &GPUTape{
		backend:    b,
		operations: make([]gpuOperation, 0, 64), // Pre-allocate for common case
		enabled:    true,
	}
}

// Record records an operation for backward pass.
// The backward function should compute gradients for all inputs given the output gradient.
func (tape *GPUTape) Record(name string, inputs []*GPUTensor, output *GPUTensor, backward func(*GPUTensor) []*GPUTensor) {
	if !tape.enabled {
		return
	}

	tape.operations = append(tape.operations, gpuOperation{
		name:     name,
		inputs:   inputs,
		output:   output,
		backward: backward,
	})
}

// Backward computes gradients for all inputs by walking the tape in reverse.
// All operations stay on GPU - no CPU transfers occur.
//
// Algorithm:
//  1. Start with loss gradient (typically ones for scalar loss)
//  2. Walk operations in reverse order
//  3. For each operation, compute input gradients using chain rule
//  4. Accumulate gradients when the same tensor is used multiple times
//
// Returns a map from GPUTensor to its accumulated gradient (also GPUTensor).
func (tape *GPUTape) Backward(loss *GPUTensor) map[*GPUTensor]*GPUTensor {
	if len(tape.operations) == 0 {
		return make(map[*GPUTensor]*GPUTensor)
	}

	// Stop recording during backward pass
	wasEnabled := tape.enabled
	tape.enabled = false
	defer func() {
		tape.enabled = wasEnabled
	}()

	// Map to accumulate gradients for each tensor
	grads := make(map[*GPUTensor]*GPUTensor)

	// Initialize with loss gradient
	lastOp := tape.operations[len(tape.operations)-1]
	grads[lastOp.output] = loss

	// Walk tape backwards
	for i := len(tape.operations) - 1; i >= 0; i-- {
		op := tape.operations[i]

		// Get gradient for this operation's output
		outputGrad, hasGrad := grads[op.output]
		if !hasGrad {
			continue // No gradient flows to this operation
		}

		// Compute input gradients
		inputGrads := op.backward(outputGrad)

		// Accumulate gradients for each input. When a gradient already exists
		// (fan-out in the computation graph), sum the partial gradients and
		// release the old intermediate buffer immediately.
		for j, input := range op.inputs {
			if j >= len(inputGrads) || inputGrads[j] == nil {
				continue
			}

			if existing, ok := grads[input]; ok {
				// Accumulate: grad += inputGrad (on GPU!)
				newGrad := tape.backend.AddGPU(existing, inputGrads[j])
				existing.Release() // Release old intermediate gradient buffer.
				grads[input] = newGrad
			} else {
				grads[input] = inputGrads[j]
			}
		}
	}

	return grads
}

// Clear resets the tape, removing all recorded operations.
// Recording state is preserved.
func (tape *GPUTape) Clear() {
	tape.operations = tape.operations[:0]
}

// Enable enables operation recording.
func (tape *GPUTape) Enable() {
	tape.enabled = true
}

// Disable disables operation recording.
func (tape *GPUTape) Disable() {
	tape.enabled = false
}

// IsEnabled returns true if the tape is currently recording operations.
func (tape *GPUTape) IsEnabled() bool {
	return tape.enabled
}

// NumOps returns the number of recorded operations.
func (tape *GPUTape) NumOps() int {
	return len(tape.operations)
}

// GPU-native backward operations.
// These compute gradients entirely on GPU without CPU transfers.

// AddBackwardGPU computes gradients for element-wise addition.
// d(a+b)/da = 1, d(a+b)/db = 1.
func (b *Backend) AddBackwardGPU(_, _, grad *GPUTensor) (*GPUTensor, *GPUTensor) {
	// For addition: gradients flow equally to both inputs
	// TODO: Handle broadcasting (reduce along broadcast dims)
	return grad, grad
}

// SubBackwardGPU computes gradients for element-wise subtraction.
// d(a-b)/da = 1, d(a-b)/db = -1.
func (b *Backend) SubBackwardGPU(_, _, grad *GPUTensor) (*GPUTensor, *GPUTensor) {
	// grad_a = grad
	gradA := grad

	// grad_b = -grad (negate on GPU). negOne is cache-owned — do not release.
	negOne := b.scalarGPU(-1.0, grad.shape, grad.dtype)
	gradB := b.MulGPU(grad, negOne)

	return gradA, gradB
}

// MulBackwardGPU computes gradients for element-wise multiplication.
// d(a*b)/da = b, d(a*b)/db = a.
func (b *Backend) MulBackwardGPU(a, c, grad *GPUTensor) (*GPUTensor, *GPUTensor) {
	// grad_a = grad * b
	gradA := b.MulGPU(grad, c)

	// grad_b = grad * a
	gradB := b.MulGPU(grad, a)

	return gradA, gradB
}

// DivBackwardGPU computes gradients for element-wise division.
// d(a/b)/da = 1/b, d(a/b)/db = -a/b^2.
func (b *Backend) DivBackwardGPU(a, c, grad *GPUTensor) (*GPUTensor, *GPUTensor) {
	// grad_a = grad / b
	gradA := b.DivGPU(grad, c)

	// grad_b = -grad * a / (b * b). negGrad is cache-owned — do not release.
	negGrad := b.scalarGPU(-1.0, grad.shape, grad.dtype)
	temp1 := b.MulGPU(grad, negGrad)
	defer temp1.Release()
	temp2 := b.MulGPU(temp1, a)
	defer temp2.Release()
	bSquared := b.MulGPU(c, c)
	defer bSquared.Release()
	gradB := b.DivGPU(temp2, bSquared)

	return gradA, gradB
}

// MatMulBackwardGPU computes gradients for matrix multiplication.
// d(A@B)/dA = grad@B^T, d(A@B)/dB = A^T@grad.
func (b *Backend) MatMulBackwardGPU(a, c, grad *GPUTensor) (*GPUTensor, *GPUTensor) {
	// grad_A = grad @ B^T
	bT := b.TransposeGPU(c)
	defer bT.Release()
	gradA := b.MatMulGPU(grad, bT)

	// grad_B = A^T @ grad
	aT := b.TransposeGPU(a)
	defer aT.Release()
	gradB := b.MatMulGPU(aT, grad)

	return gradA, gradB
}

// ReLUBackwardGPU computes gradients for ReLU activation.
// d(ReLU(x))/dx = 1 if x > 0, else 0.
// grad_input = grad * (input > 0).
func (b *Backend) ReLUBackwardGPU(input, grad *GPUTensor) *GPUTensor {
	// Validate shapes match
	if !input.shape.Equal(grad.shape) {
		panic(fmt.Sprintf("webgpu: ReLUBackwardGPU: shape mismatch: %v vs %v", input.shape, grad.shape))
	}

	numElements := input.NumElements()

	shader := b.compileShader("reluBackward", reluBackwardShader)
	entry := b.getOrCreatePipeline("reluBackward", shader, bglBinary)

	// Create output buffer
	resultSize := input.ByteSize()
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: ReLUBackwardGPU: failed to create result buffer: %v", err))
	}

	// Create uniform buffer for params
	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(input.buffer, input.bufferSize),
		bufBinding(grad.buffer, grad.bufferSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	b.execComputePass(entry.pipeline, bg, workgroups, 1, 1)

	return &GPUTensor{
		buffer:     bufferResult,
		shape:      input.shape,
		dtype:      input.dtype,
		strides:    input.strides,
		backend:    b,
		computed:   true,
		bufferSize: resultSize,
	}
}

// SigmoidBackwardGPU computes gradients for sigmoid activation.
// d(sigmoid(x))/dx = sigmoid(x) * (1 - sigmoid(x)).
func (b *Backend) SigmoidBackwardGPU(output, grad *GPUTensor) *GPUTensor {
	// grad_input = grad * output * (1 - output). one is cache-owned — do not release.
	one := b.scalarGPU(1.0, output.shape, output.dtype)
	oneMinusOutput := b.SubGPU(one, output)
	defer oneMinusOutput.Release()
	temp := b.MulGPU(output, oneMinusOutput)
	defer temp.Release()
	return b.MulGPU(grad, temp)
}

// TanhBackwardGPU computes gradients for tanh activation.
// d(tanh(x))/dx = 1 - tanh(x)^2.
func (b *Backend) TanhBackwardGPU(output, grad *GPUTensor) *GPUTensor {
	// grad_input = grad * (1 - output^2). one is cache-owned — do not release.
	one := b.scalarGPU(1.0, output.shape, output.dtype)
	outputSquared := b.MulGPU(output, output)
	defer outputSquared.Release()
	oneMinusSquared := b.SubGPU(one, outputSquared)
	defer oneMinusSquared.Release()
	return b.MulGPU(grad, oneMinusSquared)
}

// SoftmaxBackwardGPU computes gradients for softmax activation.
// d_input[i] = s[i] * (grad[i] - sum(s * grad))
// where s = softmax output.
func (b *Backend) SoftmaxBackwardGPU(output, grad *GPUTensor, dim int) *GPUTensor {
	shape := output.shape
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	if dim < 0 || dim >= ndim {
		panic("webgpu: SoftmaxBackwardGPU: dimension out of range")
	}

	// For now, only support last dimension
	if dim != ndim-1 {
		panic("webgpu: SoftmaxBackwardGPU: only last dimension supported for now")
	}

	// Validate shapes match
	if !output.shape.Equal(grad.shape) {
		panic(fmt.Sprintf("webgpu: SoftmaxBackwardGPU: shape mismatch: %v vs %v", output.shape, grad.shape))
	}

	// Calculate batch size and feature size
	batchSize := 1
	for i := 0; i < ndim-1; i++ {
		batchSize *= shape[i]
	}
	featureSize := shape[ndim-1]

	shader := b.compileShader("softmaxBackward", softmaxBackwardShader)
	entry := b.getOrCreatePipeline("softmaxBackward", shader, bglBinary)

	// Create output buffer
	resultSize := output.ByteSize()
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: SoftmaxBackwardGPU: failed to create result buffer: %v", err))
	}

	// Create uniform buffer for params
	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(batchSize))
	binary.LittleEndian.PutUint32(params[4:8], uint32(featureSize)) //nolint:gosec // G115
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(output.buffer, output.bufferSize),
		bufBinding(grad.buffer, grad.bufferSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	b.execComputePass(entry.pipeline, bg, uint32(batchSize), 1, 1)

	return &GPUTensor{
		buffer:     bufferResult,
		shape:      output.shape,
		dtype:      output.dtype,
		strides:    output.strides,
		backend:    b,
		computed:   true,
		bufferSize: resultSize,
	}
}

// SumDimGPU computes sum along the last dimension.
// Input: [batch, dim], Output: [batch].
func (b *Backend) SumDimGPU(t *GPUTensor, dim int, keepDim bool) *GPUTensor {
	shape := t.shape
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	if dim < 0 || dim >= ndim {
		panic("webgpu: SumDimGPU: dimension out of range")
	}

	// For now, only support last dimension
	if dim != ndim-1 {
		panic("webgpu: SumDimGPU: only last dimension supported for now")
	}

	// Calculate batch size and feature size
	batchSize := 1
	for i := 0; i < ndim-1; i++ {
		batchSize *= shape[i]
	}
	featureSize := shape[ndim-1]

	shader := b.compileShader("sumDim", sumDimShader)
	entry := b.getOrCreatePipeline("sumDim", shader, bglUnary)

	// Build output shape
	var outShape tensor.Shape
	if keepDim {
		outShape = make(tensor.Shape, ndim)
		copy(outShape, shape)
		outShape[dim] = 1
	} else {
		outShape = shape[:ndim-1]
		if len(outShape) == 0 {
			outShape = tensor.Shape{1}
		}
	}

	resultSize := uint64(batchSize * t.dtype.Size()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: SumDimGPU: failed to create result buffer: %v", err))
	}

	// Create uniform buffer for params
	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(batchSize))
	binary.LittleEndian.PutUint32(params[4:8], uint32(featureSize)) //nolint:gosec // G115
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(t.buffer, t.bufferSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((batchSize + workgroupSize - 1) / workgroupSize)
	b.execComputePass(entry.pipeline, bg, workgroups, 1, 1)

	return &GPUTensor{
		buffer:     bufferResult,
		shape:      outShape,
		dtype:      t.dtype,
		strides:    outShape.ComputeStrides(),
		backend:    b,
		computed:   true,
		bufferSize: resultSize,
	}
}

// scalarCacheKey encodes (value, numElements, dtype) into a single uint64 for
// use as a map key. Top 32 bits hold the float32 bit pattern; bottom 32 bits
// hold numElements XOR-masked with the dtype in the upper byte.
//
//nolint:gosec // G115: numElements and dtype are small positive integers
func scalarCacheKey(value float32, numElements int, dtype tensor.DataType) uint64 {
	valBits := uint64(math.Float32bits(value)) << 32
	elemBits := uint64(uint32(numElements) ^ (uint32(dtype) << 24))
	return valBits | elemBits
}

// scalarGPU returns a GPU tensor filled with the given constant value and
// matching the shape and dtype of a reference tensor. Results are cached by
// (value, numElements, dtype) so that repeated backward ops (e.g. -1.0 in
// SubBackward, 1.0 in SigmoidBackward) upload the constant only once per
// unique shape+dtype combination.
//
// IMPORTANT: callers must NOT call Release on the returned tensor — it is
// owned by the cache and will be released in Backend.Release(). Use defer
// only on tensors returned by ops that create new buffers (MulGPU etc.).
func (b *Backend) scalarGPU(value float32, shape tensor.Shape, dtype tensor.DataType) *GPUTensor {
	numElements := shape.NumElements()
	key := scalarCacheKey(value, numElements, dtype)

	b.scalarCacheMu.RLock()
	if cached, ok := b.scalarCache[key]; ok {
		b.scalarCacheMu.RUnlock()
		return cached
	}
	b.scalarCacheMu.RUnlock()

	// Cache miss — allocate and upload.
	raw, err := tensor.NewRaw(shape, dtype, tensor.CPU)
	if err != nil {
		panic(fmt.Sprintf("webgpu: scalarGPU: failed to create raw tensor: %v", err))
	}

	switch dtype {
	case tensor.Float32:
		data := raw.AsFloat32()
		for i := 0; i < numElements; i++ {
			data[i] = value
		}
	case tensor.Float64:
		data := raw.AsFloat64()
		for i := 0; i < numElements; i++ {
			data[i] = float64(value)
		}
	case tensor.Int32:
		data := raw.AsInt32()
		for i := 0; i < numElements; i++ {
			data[i] = int32(value)
		}
	default:
		panic(fmt.Sprintf("webgpu: scalarGPU: unsupported dtype: %v", dtype))
	}

	t := b.UploadTensor(raw)

	b.scalarCacheMu.Lock()
	// Check again under write lock in case another goroutine raced us.
	if existing, ok := b.scalarCache[key]; ok {
		b.scalarCacheMu.Unlock()
		t.Release() // Discard the duplicate we just uploaded.
		return existing
	}
	b.scalarCache[key] = t
	b.scalarCacheMu.Unlock()

	return t
}
