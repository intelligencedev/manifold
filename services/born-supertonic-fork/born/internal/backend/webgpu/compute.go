//go:build windows || linux

package webgpu

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
	"github.com/gogpu/gputypes"
	wgpu "github.com/gogpu/wgpu"
)

// compileShader compiles WGSL shader code into a ShaderModule.
// Results are cached in the Backend's shaders map.
// Panics on failure because all shaders are statically embedded and compilation
// failure indicates a programming error, not a runtime condition.
func (b *Backend) compileShader(name, code string) *wgpu.ShaderModule {
	b.mu.RLock()
	if shader, exists := b.shaders[name]; exists {
		b.mu.RUnlock()
		return shader
	}
	b.mu.RUnlock()

	// Compile shader via CreateShaderModule with WGSL source.
	shader, err := b.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: name,
		WGSL:  code,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: failed to compile shader %q: %v", name, err))
	}

	b.mu.Lock()
	b.shaders[name] = shader
	b.mu.Unlock()

	return shader
}

// getOrCreatePipeline returns a cached compute pipeline entry (pipeline + bind group layout).
// entries describes the binding layout for group 0. In gogpu/wgpu, auto-layout is not
// reflected back from the pipeline, so we create the BGL explicitly from entries and
// store it alongside the pipeline for use in CreateBindGroup calls.
//
// Panics on failure because pipelines use statically embedded shaders.
func (b *Backend) getOrCreatePipeline(name string, shader *wgpu.ShaderModule, entries []gputypes.BindGroupLayoutEntry) pipelineEntry {
	b.mu.RLock()
	if entry, exists := b.pipelines[name]; exists {
		b.mu.RUnlock()
		return entry
	}
	b.mu.RUnlock()

	// Create bind group layout for group 0.
	bgl, err := b.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   name + " BGL",
		Entries: entries,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: failed to create bind group layout for %q: %v", name, err))
	}

	// Create pipeline layout with the single bind group layout.
	pipelineLayout, err := b.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            name + " Layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		bgl.Release()
		panic(fmt.Sprintf("webgpu: failed to create pipeline layout for %q: %v", name, err))
	}

	pipeline, err := b.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      name,
		Layout:     pipelineLayout,
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil {
		pipelineLayout.Release()
		bgl.Release()
		panic(fmt.Sprintf("webgpu: failed to create compute pipeline %q: %v", name, err))
	}

	entry := pipelineEntry{pipeline: pipeline, layout: bgl, pipelineLayout: pipelineLayout}
	b.mu.Lock()
	b.pipelines[name] = entry
	b.mu.Unlock()

	return entry
}

// bglStorage returns a compute-visible storage buffer binding layout entry.
// readOnly controls whether it's read-only or read-write storage.
func bglStorage(binding uint32, readOnly bool) gputypes.BindGroupLayoutEntry {
	bufType := gputypes.BufferBindingTypeStorage
	if readOnly {
		bufType = gputypes.BufferBindingTypeReadOnlyStorage
	}
	return gputypes.BindGroupLayoutEntry{
		Binding:    binding,
		Visibility: gputypes.ShaderStageCompute,
		Buffer:     &gputypes.BufferBindingLayout{Type: bufType},
	}
}

// bglUniform returns a compute-visible uniform buffer binding layout entry.
func bglUniform(binding uint32) gputypes.BindGroupLayoutEntry {
	return gputypes.BindGroupLayoutEntry{
		Binding:    binding,
		Visibility: gputypes.ShaderStageCompute,
		Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
	}
}

// createBindGroup creates a BindGroup from a layout and a list of buffer bindings.
// Each entry in bufs is {buffer, offset, size} mapping to sequential bindings 0..N.
// The last entry is always the uniform params buffer.
//
// Panics on failure since all bind groups use validated pipeline layouts.
func (b *Backend) createBindGroupFromBuffers(layout *wgpu.BindGroupLayout, bufs []bindGroupBuffer) *wgpu.BindGroup {
	entries := make([]wgpu.BindGroupEntry, len(bufs))
	for i, buf := range bufs {
		entries[i] = wgpu.BindGroupEntry{
			Binding: uint32(i),
			Buffer:  buf.buffer,
			Offset:  buf.offset,
			Size:    buf.size,
		}
	}
	bg, err := b.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout:  layout,
		Entries: entries,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: failed to create bind group: %v", err))
	}
	return bg
}

// bindGroupBuffer describes a single buffer binding for createBindGroupFromBuffers.
type bindGroupBuffer struct {
	buffer *wgpu.Buffer
	offset uint64
	size   uint64
}

// bufBinding is a shorthand to create a bindGroupBuffer entry (0-offset, full size).
func bufBinding(buf *wgpu.Buffer, size uint64) bindGroupBuffer {
	return bindGroupBuffer{buffer: buf, offset: 0, size: size}
}

// execComputePass encodes a single compute pass and submits it.
// Used for GPU-resident operations (gpu_ops.go, gpu_autodiff.go) where the result
// stays on GPU in a storage buffer for subsequent GPU operations.
// When the result eventually needs to be read back, call readBuffer() which
// uses Poll(PollWait) + a separate copy encoder to safely transfer the data.
//
// Panics on failure since compute pass errors indicate a programming error.
//
//nolint:unparam // z is always 1 currently but kept for future 3D workgroup dispatch support
func (b *Backend) execComputePass(pipeline *wgpu.ComputePipeline, bg *wgpu.BindGroup, x, y, z uint32) {
	encoder, err := b.device.CreateCommandEncoder(nil)
	if err != nil {
		panic(fmt.Sprintf("webgpu: failed to create command encoder: %v", err))
	}

	computePass, err := encoder.BeginComputePass(nil)
	if err != nil {
		encoder.DiscardEncoding()
		panic(fmt.Sprintf("webgpu: failed to begin compute pass: %v", err))
	}

	computePass.SetPipeline(pipeline)
	computePass.SetBindGroup(0, bg, nil)
	computePass.Dispatch(x, y, z)
	if err := computePass.End(); err != nil {
		encoder.DiscardEncoding()
		panic(fmt.Sprintf("webgpu: compute pass end error: %v", err))
	}

	cmdBuffer, err := encoder.Finish()
	if err != nil {
		panic(fmt.Sprintf("webgpu: encoder finish error: %v", err))
	}
	if _, err := b.queue.Submit(cmdBuffer); err != nil {
		panic(fmt.Sprintf("webgpu: queue submit error: %v", err))
	}
}

// execComputeAndRead runs a compute pass and copies the result to CPU in a SINGLE encoder.
// gogpu/wgpu requires that CopyBufferToBuffer and the compute pass that writes
// the source buffer be submitted in the same command buffer — separate submits
// leave the source buffer in an undefined state on some drivers (DX12, Vulkan).
//
// The function creates a temporary staging buffer, encodes
//
//	compute_pass → CopyBufferToBuffer(resultBuf → staging) → Finish → Submit → Map
//
// all in one encoder, then returns the mapped bytes.
//
// Panics on failure because all callers use statically validated buffers.
func (b *Backend) execComputeAndRead(
	pipeline *wgpu.ComputePipeline,
	bg *wgpu.BindGroup,
	x, y, z uint32,
	resultBuf *wgpu.Buffer,
	resultSize uint64,
) []byte {
	// Flush any active encoder batch before issuing a synchronous submit.
	// Without this, the active encoder's resources may still reference buffers
	// that we are about to read or reuse, causing validation errors.
	b.flushCommands()

	// Create staging buffer for readback (MapRead | CopyDst).
	stagingBuf, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst | gputypes.BufferUsageCopySrc,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: execComputeAndRead: failed to create staging buffer: %v", err))
	}
	defer stagingBuf.Release()

	// Single encoder: compute pass + copy to staging.
	encoder, err := b.device.CreateCommandEncoder(nil)
	if err != nil {
		panic(fmt.Sprintf("webgpu: execComputeAndRead: failed to create command encoder: %v", err))
	}

	computePass, err := encoder.BeginComputePass(nil)
	if err != nil {
		encoder.DiscardEncoding()
		panic(fmt.Sprintf("webgpu: execComputeAndRead: failed to begin compute pass: %v", err))
	}

	computePass.SetPipeline(pipeline)
	computePass.SetBindGroup(0, bg, nil)
	computePass.Dispatch(x, y, z)
	if err := computePass.End(); err != nil {
		encoder.DiscardEncoding()
		panic(fmt.Sprintf("webgpu: execComputeAndRead: compute pass end error: %v", err))
	}

	// CopyBufferToBuffer INSIDE the same encoder, after pass.End(), before Finish().
	encoder.CopyBufferToBuffer(resultBuf, 0, stagingBuf, 0, resultSize)

	cmdBuffer, err := encoder.Finish()
	if err != nil {
		panic(fmt.Sprintf("webgpu: execComputeAndRead: encoder finish error: %v", err))
	}
	if _, err := b.queue.Submit(cmdBuffer); err != nil {
		panic(fmt.Sprintf("webgpu: execComputeAndRead: queue submit error: %v", err))
	}

	// Map staging buffer. Map() blocks until the GPU fence resolves.
	if err := stagingBuf.Map(context.Background(), wgpu.MapModeRead, 0, resultSize); err != nil {
		panic(fmt.Sprintf("webgpu: execComputeAndRead: failed to map staging buffer: %v", err))
	}
	defer func() { _ = stagingBuf.Unmap() }()

	mappedRange, err := stagingBuf.MappedRange(0, resultSize)
	if err != nil {
		panic(fmt.Sprintf("webgpu: execComputeAndRead: failed to get mapped range: %v", err))
	}
	defer mappedRange.Release()

	out := make([]byte, resultSize)
	copy(out, mappedRange.Bytes())
	return out
}

// createBuffer creates a GPU buffer and uploads initial data via MappedAtCreation.
// Panics on failure because all buffers use validated sizes from tensor data.
func (b *Backend) createBuffer(data []byte, usage gputypes.BufferUsage) *wgpu.Buffer {
	size := uint64(len(data))

	buffer, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage:            usage,
		Size:             size,
		MappedAtCreation: true,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: failed to create buffer (size=%d): %v", size, err))
	}

	// MappedAtCreation makes the buffer immediately available for writing.
	// MappedRange offset must be a multiple of 8, size a multiple of 4.
	mappedRange, err := buffer.MappedRange(0, size)
	if err != nil {
		buffer.Release()
		panic(fmt.Sprintf("webgpu: failed to get mapped range: %v", err))
	}
	copy(mappedRange.Bytes(), data)
	mappedRange.Release()

	if err := buffer.Unmap(); err != nil {
		buffer.Release()
		panic(fmt.Sprintf("webgpu: failed to unmap buffer: %v", err))
	}

	return buffer
}

// createUniformBuffer creates a uniform buffer with 16-byte alignment.
// Panics on failure.
func (b *Backend) createUniformBuffer(data []byte) *wgpu.Buffer {
	// Ensure 16-byte alignment required by uniform buffers.
	size := uint64(len(data))
	alignedSize := (size + 15) &^ 15

	buffer, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage:            gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		Size:             alignedSize,
		MappedAtCreation: true,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: failed to create uniform buffer: %v", err))
	}

	mappedRange, err := buffer.MappedRange(0, alignedSize)
	if err != nil {
		buffer.Release()
		panic(fmt.Sprintf("webgpu: failed to get uniform mapped range: %v", err))
	}
	copy(mappedRange.Bytes(), data)
	mappedRange.Release()

	if err := buffer.Unmap(); err != nil {
		buffer.Release()
		panic(fmt.Sprintf("webgpu: failed to unmap uniform buffer: %v", err))
	}

	return buffer
}

// readBuffer reads data from a GPU storage buffer back to CPU.
// Used by GPUTensor.ToCPU() for GPU-resident tensors produced by gpu_ops.go.
//
// This function uses a dedicated staging buffer approach:
//  1. Poll(PollWait) to ensure all prior GPU work (compute passes) is complete.
//  2. New encoder: CopyBufferToBuffer(srcStorage → staging).
//  3. Submit + Map staging buffer.
//
// This is the "split encoder" path — it works because Poll(PollWait) guarantees
// the compute pass has fully committed its writes before the copy encoder runs.
//
// For non-lazy operations (runBinaryOp, runUnaryOp, etc.) use execComputeAndRead()
// instead, which keeps compute + copy in a single encoder without needing a poll.
func (b *Backend) readBuffer(srcBuffer *wgpu.Buffer, size uint64) ([]byte, error) {
	// Flush all pending lazy-mode command buffers.
	b.flushCommands()

	// Wait for ALL pending GPU work to complete before reading.
	// This ensures the compute pass has written its results to srcBuffer
	// before we issue the CopyBufferToBuffer in a new encoder.
	b.device.Poll(wgpu.PollWait)

	// Create staging buffer for reading (MAP_READ | COPY_DST).
	stagingBuffer, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst | gputypes.BufferUsageCopySrc,
		Size:  size,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: failed to create staging buffer: %w", err)
	}
	defer stagingBuffer.Release()

	// Copy from GPU storage buffer to staging buffer.
	encoder, err := b.device.CreateCommandEncoder(nil)
	if err != nil {
		return nil, fmt.Errorf("webgpu: failed to create command encoder: %w", err)
	}
	encoder.CopyBufferToBuffer(srcBuffer, 0, stagingBuffer, 0, size)
	cmdBuffer, err := encoder.Finish()
	if err != nil {
		return nil, fmt.Errorf("webgpu: encoder finish: %w", err)
	}
	if _, err = b.queue.Submit(cmdBuffer); err != nil {
		return nil, fmt.Errorf("webgpu: queue submit: %w", err)
	}

	// Map staging buffer for reading. Map() blocks until the GPU fence resolves.
	if err = stagingBuffer.Map(context.Background(), wgpu.MapModeRead, 0, size); err != nil {
		return nil, fmt.Errorf("webgpu: failed to map staging buffer: %w", err)
	}
	defer func() { _ = stagingBuffer.Unmap() }()

	// MappedRange returns a safe view over the mapped region.
	mappedRange, err := stagingBuffer.MappedRange(0, size)
	if err != nil {
		return nil, fmt.Errorf("webgpu: failed to get mapped range: %w", err)
	}
	defer mappedRange.Release()

	result := make([]byte, size)
	copy(result, mappedRange.Bytes())

	return result, nil
}

// bglBinary returns BGL entries for binary ops: 2 RO storage + 1 RW storage + 1 uniform.
var bglBinary = []gputypes.BindGroupLayoutEntry{
	bglStorage(0, true), bglStorage(1, true), bglStorage(2, false), bglUniform(3),
}

// bglUnary returns BGL entries for unary ops: 1 RO storage + 1 RW storage + 1 uniform.
var bglUnary = []gputypes.BindGroupLayoutEntry{
	bglStorage(0, true), bglStorage(1, false), bglUniform(2),
}

// bglWhere returns BGL entries for where op: 3 RO storage + 1 RW storage + 1 uniform.
var bglWhere = []gputypes.BindGroupLayoutEntry{
	bglStorage(0, true), bglStorage(1, true), bglStorage(2, true), bglStorage(3, false), bglUniform(4),
}

// bglScatter returns BGL entries for scatter-add ops: 3 RO storage + 1 RW storage + 1 uniform.
// Identical layout to bglWhere: dest(RO), indices(RO), src(RO), result(RW), params(uniform).
var bglScatter = []gputypes.BindGroupLayoutEntry{
	bglStorage(0, true), bglStorage(1, true), bglStorage(2, true), bglStorage(3, false), bglUniform(4),
}

// runBinaryOp executes a binary element-wise operation (add, sub, mul, div) on GPU.
// Supports NumPy-style broadcasting. Supports float32 and int32 dtypes.
func (b *Backend) runBinaryOp(a, other *tensor.RawTensor, shaderName, shaderCode string) (*tensor.RawTensor, error) {
	// Validate inputs - must have same dtype
	if a.DType() != other.DType() {
		return nil, fmt.Errorf("webgpu: dtype mismatch: %s vs %s", a.DType(), other.DType())
	}

	// Only float32 and int32 are supported
	dtype := a.DType()
	if dtype != tensor.Float32 && dtype != tensor.Int32 {
		return nil, fmt.Errorf("webgpu: only float32 and int32 are supported, got %s", dtype)
	}

	// Handle broadcasting if shapes don't match
	if !a.Shape().Equal(other.Shape()) {
		broadcastedShape, ok, _ := tensor.BroadcastShapes(a.Shape(), other.Shape())
		if !ok {
			return nil, fmt.Errorf("webgpu: shapes not broadcastable: %v vs %v", a.Shape(), other.Shape())
		}
		if !a.Shape().Equal(broadcastedShape) {
			a = b.Expand(a, broadcastedShape)
		}
		if !other.Shape().Equal(broadcastedShape) {
			other = b.Expand(other, broadcastedShape)
		}
	}

	numElements := a.NumElements()

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglBinary)

	bufferA := b.createBuffer(a.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferA.Release()

	bufferOther := b.createBuffer(other.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferOther.Release()

	resultSize := uint64(a.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferA, resultSize),
		bufBinding(bufferOther, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(a.Shape(), a.DType(), tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runComparisonOp executes a binary comparison operation (greater, lower, equal, etc.) on GPU.
// Always returns float32 result (0.0 for false, 1.0 for true).
// Converts int32 inputs to float32 before comparison.
func (b *Backend) runComparisonOp(a, other *tensor.RawTensor, shaderName, shaderCode string) (*tensor.RawTensor, error) {
	// Validate inputs - must have same dtype
	if a.DType() != other.DType() {
		return nil, fmt.Errorf("webgpu: dtype mismatch: %s vs %s", a.DType(), other.DType())
	}

	// Only float32 and int32 are supported
	dtype := a.DType()
	if dtype != tensor.Float32 && dtype != tensor.Int32 {
		return nil, fmt.Errorf("webgpu: only float32 and int32 are supported, got %s", dtype)
	}

	// Convert int32 to float32 for comparison shaders (they only support f32)
	if dtype == tensor.Int32 {
		var err error
		a, err = int32ToFloat32(a)
		if err != nil {
			return nil, err
		}
		other, err = int32ToFloat32(other)
		if err != nil {
			return nil, err
		}
	}

	// Handle broadcasting if shapes don't match
	if !a.Shape().Equal(other.Shape()) {
		broadcastedShape, ok, _ := tensor.BroadcastShapes(a.Shape(), other.Shape())
		if !ok {
			return nil, fmt.Errorf("webgpu: shapes not broadcastable: %v vs %v", a.Shape(), other.Shape())
		}
		if !a.Shape().Equal(broadcastedShape) {
			a = b.Expand(a, broadcastedShape)
		}
		if !other.Shape().Equal(broadcastedShape) {
			other = b.Expand(other, broadcastedShape)
		}
	}

	numElements := a.NumElements()

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglBinary)

	bufferA := b.createBuffer(a.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferA.Release()

	bufferOther := b.createBuffer(other.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferOther.Release()

	resultSize := uint64(a.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferA, resultSize),
		bufBinding(bufferOther, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	// Comparison always returns float32 (0.0/1.0).
	result, err := tensor.NewRaw(a.Shape(), tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runUnaryOp executes a unary element-wise operation (relu, sigmoid, tanh, neg, exp, log, sqrt) on GPU.
func (b *Backend) runUnaryOp(input *tensor.RawTensor, shaderName, shaderCode string) (*tensor.RawTensor, error) {
	if input.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: only float32 is supported, got %s", input.DType())
	}

	numElements := input.NumElements()

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(input.Shape(), input.DType(), tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runMatMul executes matrix multiplication C = A @ B on GPU.
// A is [M, K], B is [K, N], C is [M, N].
func (b *Backend) runMatMul(a, other *tensor.RawTensor) (*tensor.RawTensor, error) {
	// Validate inputs
	if a.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: only float32 is supported, got %s", a.DType())
	}
	if len(a.Shape()) != 2 || len(other.Shape()) != 2 {
		return nil, fmt.Errorf("webgpu: matmul requires 2D tensors, got %v and %v", a.Shape(), other.Shape())
	}

	M := uint32(a.Shape()[0]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	K := uint32(a.Shape()[1]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	N := uint32(other.Shape()[1]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	if other.Shape()[0] != int(K) {
		return nil, fmt.Errorf("webgpu: matmul shape mismatch: [%d,%d] @ [%d,%d]", M, K, other.Shape()[0], N)
	}

	shader := b.compileShader("matmul", matmulShader)
	entry := b.getOrCreatePipeline("matmul", shader, bglBinary)

	bufferA := b.createBuffer(a.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferA.Release()

	bufferOther := b.createBuffer(other.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferOther.Release()

	resultShape := tensor.Shape{int(M), int(N)}
	resultSize := uint64(int(M) * int(N) * 4) //nolint:gosec // float32 = 4 bytes
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	// Uniform: M, K, N as u32 (3×4 = 12 bytes, padded to 16).
	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], M)
	binary.LittleEndian.PutUint32(params[4:8], K)
	binary.LittleEndian.PutUint32(params[8:12], N)
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	aSize := uint64(a.ByteSize())         //nolint:gosec // G115: integer overflow conversion int -> uint64
	otherSize := uint64(other.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferA, aSize),
		bufBinding(bufferOther, otherSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	// 2D workgroup dispatch: 16×16 tiles.
	workgroupsX := uint32(math.Ceil(float64(N) / 16.0))
	workgroupsY := uint32(math.Ceil(float64(M) / 16.0))
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroupsX, workgroupsY, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(resultShape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runTranspose executes 2D matrix transpose on GPU.
func (b *Backend) runTranspose(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	// Validate input
	if input.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: only float32 is supported, got %s", input.DType())
	}
	if len(input.Shape()) != 2 {
		return nil, fmt.Errorf("webgpu: transpose requires 2D tensor, got %v", input.Shape())
	}

	rows := uint32(input.Shape()[0]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	cols := uint32(input.Shape()[1]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	shader := b.compileShader("transpose", transposeShader)
	entry := b.getOrCreatePipeline("transpose", shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	// Uniform: rows, cols as u32.
	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], rows)
	binary.LittleEndian.PutUint32(params[4:8], cols)
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	// 2D workgroup dispatch: 16×16 tiles.
	workgroupsX := uint32(math.Ceil(float64(cols) / 16.0))
	workgroupsY := uint32(math.Ceil(float64(rows) / 16.0))
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroupsX, workgroupsY, 1, bufferResult, resultSize)

	resultShape := tensor.Shape{int(cols), int(rows)}
	result, err := tensor.NewRaw(resultShape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runClamp executes element-wise clamping: clamp(x, min, max) on GPU.
// Supports float32 and int32 dtypes.
func (b *Backend) runClamp(input *tensor.RawTensor, minBound, maxBound any) (*tensor.RawTensor, error) {
	dtype := input.DType()
	if dtype != tensor.Float32 && dtype != tensor.Int32 {
		return nil, fmt.Errorf("webgpu: Clamp: only float32 and int32 are supported, got %s", dtype)
	}

	numElements := input.NumElements()

	shaderName, shaderCode := selectBinaryShader(dtype, "clamp", clampShader, clampShaderInt32)

	shader := b.compileShader(shaderName, shaderCode)
	pipeline := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	params := make([]byte, 16)                                      // 16-byte aligned: size + min + max
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32

	if dtype == tensor.Float32 {
		minVal := minBound.(float32)
		maxVal := maxBound.(float32)
		binary.LittleEndian.PutUint32(params[4:8], math.Float32bits(minVal))
		binary.LittleEndian.PutUint32(params[8:12], math.Float32bits(maxVal))
	} else {
		minVal := minBound.(int32)
		maxVal := maxBound.(int32)
		binary.LittleEndian.PutUint32(params[4:8], uint32(minVal))  //nolint:gosec // G115: safe, int32 fits in uint32
		binary.LittleEndian.PutUint32(params[8:12], uint32(maxVal)) //nolint:gosec // G115: safe, int32 fits in uint32
	}
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(pipeline.layout, []bindGroupBuffer{
		bufBinding(bufferInput, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	resultData := b.execComputeAndRead(pipeline.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(input.Shape(), dtype, tensor.WebGPU)
	if err != nil {
		return nil, err
	}
	copy(result.Data(), resultData)
	return result, nil
}

// runScalarOp executes a scalar operation (MulScalar, AddScalar, etc.) on GPU.
// The shader params contain both size (u32) and scalar value (f32).
func (b *Backend) runScalarOp(input *tensor.RawTensor, scalar float32, shaderName, shaderCode string) (*tensor.RawTensor, error) {
	// Validate input
	if input.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: only float32 is supported, got %s", input.DType())
	}

	numElements := input.NumElements()

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	// Uniform: size as u32, scalar as f32.
	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	binary.LittleEndian.PutUint32(params[4:8], math.Float32bits(scalar))
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(input.Shape(), input.DType(), tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runSoftmax executes softmax along the last dimension on GPU.
// Input shape: [batch_size, num_classes].
func (b *Backend) runSoftmax(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	// Validate input
	if input.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: only float32 is supported, got %s", input.DType())
	}
	if len(input.Shape()) != 2 {
		return nil, fmt.Errorf("webgpu: softmax requires 2D tensor, got %v", input.Shape())
	}

	batchSize := uint32(input.Shape()[0]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	numClasses := uint32(input.Shape()[1]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	shader := b.compileShader("softmax", softmaxShader)
	entry := b.getOrCreatePipeline("softmax", shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	// Uniform: batch_size, num_classes as u32.
	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], batchSize)
	binary.LittleEndian.PutUint32(params[4:8], numClasses)
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	// Each workgroup handles one row (batch sample).
	workgroups := (batchSize + workgroupSize - 1) / workgroupSize
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(input.Shape(), tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runBatchMatMul executes batched matrix multiplication on GPU.
// Supports 3D [batch, M, K] @ [batch, K, N] -> [batch, M, N]
// and 4D [batch, heads, M, K] @ [batch, heads, K, N] -> [batch, heads, M, N].
func (b *Backend) runBatchMatMul(a, other *tensor.RawTensor) (*tensor.RawTensor, error) {
	// Validate inputs
	if a.DType() != tensor.Float32 || other.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: only float32 is supported")
	}

	shapeA := a.Shape()
	shapeB := other.Shape()

	if len(shapeA) != len(shapeB) || (len(shapeA) != 3 && len(shapeA) != 4) {
		return nil, fmt.Errorf("webgpu: BatchMatMul requires 3D or 4D tensors with matching dimensions")
	}

	var batch, M, K, N uint32
	var resultShape tensor.Shape

	if len(shapeA) == 3 {
		// 3D: [batch, M, K] @ [batch, K, N]

		batch = uint32(shapeA[0]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

		M = uint32(shapeA[1]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

		K = uint32(shapeA[2]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

		N = uint32(shapeB[2]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32
		resultShape = tensor.Shape{int(batch), int(M), int(N)}
	} else {
		// 4D: [batch, heads, M, K] @ [batch, heads, K, N]
		// Treat as [batch*heads, M, K] @ [batch*heads, K, N]

		batch = uint32(shapeA[0] * shapeA[1]) //nolint:gosec // G115: integer overflow conversion int -> uint32

		M = uint32(shapeA[2]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

		K = uint32(shapeA[3]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

		N = uint32(shapeB[3]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32
		resultShape = tensor.Shape{shapeA[0], shapeA[1], int(M), int(N)}
	}

	shader := b.compileShader("batchMatMul", batchMatMulShader)
	entry := b.getOrCreatePipeline("batchMatMul", shader, bglBinary)

	bufferA := b.createBuffer(a.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferA.Release()

	bufferB := b.createBuffer(other.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferB.Release()

	resultSize := uint64(batch) * uint64(M) * uint64(N) * 4 // float32 = 4 bytes
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], batch)
	binary.LittleEndian.PutUint32(params[4:8], M)
	binary.LittleEndian.PutUint32(params[8:12], K)
	binary.LittleEndian.PutUint32(params[12:16], N)
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	aSize := uint64(a.ByteSize())         //nolint:gosec // G115: integer overflow conversion int -> uint64
	otherSize := uint64(other.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferA, aSize),
		bufBinding(bufferB, otherSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	// Dispatch: (N+7)/8 × (M+7)/8 × batch.
	workgroupsX := (N + 7) / 8
	workgroupsY := (M + 7) / 8
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroupsX, workgroupsY, batch, bufferResult, resultSize)

	result, err := tensor.NewRaw(resultShape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runConv2D executes 2D convolution on GPU.
// Input shape: [batch, in_channels, height, width].
// Kernel shape: [out_channels, in_channels, kH, kW].
func (b *Backend) runConv2D(input, kernel *tensor.RawTensor, stride, padding int) (*tensor.RawTensor, error) {
	if input.DType() != tensor.Float32 || kernel.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: only float32 is supported")
	}

	inShape := input.Shape()
	kShape := kernel.Shape()

	if len(inShape) != 4 || len(kShape) != 4 {
		return nil, fmt.Errorf("webgpu: Conv2D requires 4D tensors")
	}

	batchSize := uint32(inShape[0]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	inChannels := uint32(inShape[1]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	inHeight := uint32(inShape[2]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	inWidth := uint32(inShape[3]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	outChannels := uint32(kShape[0]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	kernelH := uint32(kShape[2]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	kernelW := uint32(kShape[3]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	// Calculate output dimensions

	outHeight := (inHeight+2*uint32(padding)-kernelH)/uint32(stride) + 1 //nolint:gosec // G115: integer overflow conversion int -> uint32

	outWidth := (inWidth+2*uint32(padding)-kernelW)/uint32(stride) + 1 //nolint:gosec // G115: integer overflow conversion int -> uint32

	resultShape := tensor.Shape{int(batchSize), int(outChannels), int(outHeight), int(outWidth)}

	shader := b.compileShader("conv2d", conv2dShader)
	entry := b.getOrCreatePipeline("conv2d", shader, bglBinary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	bufferKernel := b.createBuffer(kernel.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferKernel.Release()

	resultSize := uint64(batchSize) * uint64(outChannels) * uint64(outHeight) * uint64(outWidth) * 4
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	// Uniform: 9 u32 fields (36 bytes), padded to 48.
	params := make([]byte, 48)
	binary.LittleEndian.PutUint32(params[0:4], batchSize)
	binary.LittleEndian.PutUint32(params[4:8], inChannels)
	binary.LittleEndian.PutUint32(params[8:12], inHeight)
	binary.LittleEndian.PutUint32(params[12:16], inWidth)
	binary.LittleEndian.PutUint32(params[16:20], outChannels)
	binary.LittleEndian.PutUint32(params[20:24], kernelH)
	binary.LittleEndian.PutUint32(params[24:28], kernelW)
	binary.LittleEndian.PutUint32(params[28:32], uint32(stride))  //nolint:gosec // G115: integer overflow conversion int -> uint32
	binary.LittleEndian.PutUint32(params[32:36], uint32(padding)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	inputSize := uint64(input.ByteSize())   //nolint:gosec // G115: integer overflow conversion int -> uint64
	kernelSize := uint64(kernel.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, inputSize),
		bufBinding(bufferKernel, kernelSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 48),
	})
	defer bg.Release()

	workgroupsX := (outWidth + 7) / 8
	workgroupsY := (outHeight + 7) / 8
	workgroupsZ := batchSize * outChannels
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroupsX, workgroupsY, workgroupsZ, bufferResult, resultSize)

	result, err := tensor.NewRaw(resultShape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runMaxPool2D executes 2D max pooling on GPU.
// Input shape: [batch, channels, height, width].
func (b *Backend) runMaxPool2D(input *tensor.RawTensor, kernelSize, stride int) (*tensor.RawTensor, error) {
	if input.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: only float32 is supported")
	}

	inShape := input.Shape()
	if len(inShape) != 4 {
		return nil, fmt.Errorf("webgpu: MaxPool2D requires 4D tensor")
	}

	batchSize := uint32(inShape[0]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	channels := uint32(inShape[1]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	inHeight := uint32(inShape[2]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	inWidth := uint32(inShape[3]) //nolint:gosec // G115: safe, tensor dimensions are non-negative and fit in uint32

	kSize := uint32(kernelSize) //nolint:gosec // G115: integer overflow conversion int -> uint32

	outHeight := (inHeight-kSize)/uint32(stride) + 1 //nolint:gosec // G115: integer overflow conversion int -> uint32

	outWidth := (inWidth-kSize)/uint32(stride) + 1 //nolint:gosec // G115: integer overflow conversion int -> uint32

	resultShape := tensor.Shape{int(batchSize), int(channels), int(outHeight), int(outWidth)}

	shader := b.compileShader("maxPool2d", maxPool2dShader)
	entry := b.getOrCreatePipeline("maxPool2d", shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	resultSize := uint64(batchSize) * uint64(channels) * uint64(outHeight) * uint64(outWidth) * 4
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	params := make([]byte, 32)
	binary.LittleEndian.PutUint32(params[0:4], batchSize)
	binary.LittleEndian.PutUint32(params[4:8], channels)
	binary.LittleEndian.PutUint32(params[8:12], inHeight)
	binary.LittleEndian.PutUint32(params[12:16], inWidth)
	binary.LittleEndian.PutUint32(params[16:20], kSize)
	binary.LittleEndian.PutUint32(params[20:24], kSize)
	binary.LittleEndian.PutUint32(params[24:28], uint32(stride)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	inputSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, inputSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 32),
	})
	defer bg.Release()

	workgroupsX := (outWidth + 7) / 8
	workgroupsY := (outHeight + 7) / 8
	workgroupsZ := batchSize * channels
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroupsX, workgroupsY, workgroupsZ, bufferResult, resultSize)

	result, err := tensor.NewRaw(resultShape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runSum executes global sum reduction on GPU.
// Supports float32 and int32 dtypes.
func (b *Backend) runSum(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	dtype := input.DType()
	if dtype != tensor.Float32 && dtype != tensor.Int32 {
		return nil, fmt.Errorf("webgpu: only float32 and int32 are supported, got %s", dtype)
	}

	numElements := input.NumElements()

	// For small tensors, use CPU
	if numElements < 1024 {
		return b.runSumCPU(input)
	}

	// GPU parallel reduction
	return b.runSumGPU(input)
}

// runSumCPU executes sum on CPU for small tensors.
func (b *Backend) runSumCPU(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	dtype := input.DType()

	switch dtype {
	case tensor.Float32:
		data := input.AsFloat32()
		var sum float32
		for _, v := range data {
			sum += v
		}
		result, err := tensor.NewRaw(tensor.Shape{}, tensor.Float32, tensor.WebGPU)
		if err != nil {
			return nil, err
		}
		result.AsFloat32()[0] = sum
		return result, nil

	case tensor.Int32:
		data := input.AsInt32()
		var sum int32
		for _, v := range data {
			sum += v
		}
		result, err := tensor.NewRaw(tensor.Shape{}, tensor.Int32, tensor.WebGPU)
		if err != nil {
			return nil, err
		}
		result.AsInt32()[0] = sum
		return result, nil

	default:
		return nil, fmt.Errorf("webgpu: unsupported dtype for Sum: %s", dtype)
	}
}

// runSumGPU executes sum on GPU for large tensors.
func (b *Backend) runSumGPU(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	dtype := input.DType()
	numElements := input.NumElements()

	// Select shader based on dtype
	var shaderName string
	var shaderCode string
	switch dtype {
	case tensor.Float32:
		shaderName = "globalSum"
		shaderCode = globalSumShader
	case tensor.Int32:
		shaderName = "globalSumInt32"
		shaderCode = globalSumShaderInt32
	default:
		return nil, fmt.Errorf("webgpu: unsupported dtype for Sum: %s", dtype)
	}

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	numWorkgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	partialSumsSize := uint64(numWorkgroups) * 4

	bufferPartialSums, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  partialSumsSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create partial sums buffer: %w", err)
	}
	defer bufferPartialSums.Release()

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	inputSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, inputSize),
		bufBinding(bufferPartialSums, partialSumsSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	partialData := b.execComputeAndRead(entry.pipeline, bg, numWorkgroups, 1, 1, bufferPartialSums, partialSumsSize)

	// Sum partial results on CPU based on dtype
	switch dtype {
	case tensor.Float32:
		var sum float32
		for i := uint32(0); i < numWorkgroups; i++ {
			sum += math.Float32frombits(binary.LittleEndian.Uint32(partialData[i*4 : i*4+4]))
		}
		result, err := tensor.NewRaw(tensor.Shape{}, tensor.Float32, tensor.WebGPU)
		if err != nil {
			return nil, err
		}
		result.AsFloat32()[0] = sum
		return result, nil

	case tensor.Int32:
		var sum int32
		for i := uint32(0); i < numWorkgroups; i++ {
			sum += int32(binary.LittleEndian.Uint32(partialData[i*4 : i*4+4])) //nolint:gosec // G115: integer overflow conversion uint32 -> int32
		}
		result, err := tensor.NewRaw(tensor.Shape{}, tensor.Int32, tensor.WebGPU)
		if err != nil {
			return nil, err
		}
		result.AsInt32()[0] = sum
		return result, nil

	default:
		return nil, fmt.Errorf("webgpu: unsupported dtype for Sum: %s", dtype)
	}
}

// runArgmax executes argmax along last dimension on GPU.
func (b *Backend) runArgmax(input *tensor.RawTensor, dim int) (*tensor.RawTensor, error) {
	if input.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: only float32 is supported")
	}

	shape := input.Shape()
	ndim := len(shape)

	// Normalize dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// Currently only supports last dimension
	if dim != ndim-1 {
		return nil, fmt.Errorf("webgpu: Argmax currently only supports last dimension (dim=-1)")
	}

	// Calculate batch size (product of all dimensions except last)
	batchSize := 1
	for i := 0; i < ndim-1; i++ {
		batchSize *= shape[i]
	}
	dimSize := shape[ndim-1]

	// Result shape: remove last dimension
	var resultShape tensor.Shape
	if ndim > 1 {
		resultShape = make(tensor.Shape, ndim-1)
		copy(resultShape, shape[:ndim-1])
	} else {
		resultShape = tensor.Shape{1}
	}

	shader := b.compileShader("argmax", argmaxShader)
	entry := b.getOrCreatePipeline("argmax", shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	resultSize := uint64(batchSize) * 4
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(batchSize))
	binary.LittleEndian.PutUint32(params[4:8], uint32(dimSize)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	inputSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, inputSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((batchSize + workgroupSize - 1) / workgroupSize)
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(resultShape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runEmbedding performs embedding lookup on GPU.
// weight: [num_embeddings, embedding_dim], indices: [...], output: [..., embedding_dim].
func (b *Backend) runEmbedding(weight, indices *tensor.RawTensor) (*tensor.RawTensor, error) {
	if weight.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: Embedding weight must be float32, got %s", weight.DType())
	}
	if indices.DType() != tensor.Int32 {
		return nil, fmt.Errorf("webgpu: Embedding indices must be int32, got %s", indices.DType())
	}
	if len(weight.Shape()) != 2 {
		return nil, fmt.Errorf("webgpu: Embedding weight must be 2D, got %v", weight.Shape())
	}

	numEmbeddings := weight.Shape()[0]
	embeddingDim := weight.Shape()[1]
	numIndices := indices.NumElements()

	// Output shape: [...indices_shape, embedding_dim]
	indicesShape := indices.Shape()
	outputShape := make(tensor.Shape, len(indicesShape)+1)
	copy(outputShape, indicesShape)
	outputShape[len(outputShape)-1] = embeddingDim

	shader := b.compileShader("embedding", embeddingShader)
	entry := b.getOrCreatePipeline("embedding", shader, bglBinary)

	bufferWeight := b.createBuffer(weight.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferWeight.Release()

	bufferIndices := b.createBuffer(indices.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferIndices.Release()

	resultSize := uint64(numIndices) * uint64(embeddingDim) * 4 //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numIndices))     //nolint:gosec // G115: integer overflow conversion int -> uint32
	binary.LittleEndian.PutUint32(params[4:8], uint32(embeddingDim))   //nolint:gosec // G115: safe, embedding dimensions are non-negative and fit in uint32
	binary.LittleEndian.PutUint32(params[8:12], uint32(numEmbeddings)) //nolint:gosec // G115: safe, embedding count is non-negative and fits in uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	weightSize := uint64(weight.ByteSize())   //nolint:gosec // G115: integer overflow conversion int -> uint64
	indicesSize := uint64(indices.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferWeight, weightSize),
		bufBinding(bufferIndices, indicesSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	totalElements := numIndices * embeddingDim
	workgroups := uint32((totalElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(outputShape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// boolToFloat32 converts a bool tensor to float32 for GPU operations.
// WGSL doesn't have native bool arrays, so we use float32 (0.0 = false, 1.0 = true).
func boolToFloat32(condition *tensor.RawTensor) (*tensor.RawTensor, error) {
	result, err := tensor.NewRaw(condition.Shape(), tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}
	boolData := condition.Data()
	floatData := result.AsFloat32()
	for i := 0; i < condition.NumElements(); i++ {
		if boolData[i] != 0 {
			floatData[i] = 1.0
		} else {
			floatData[i] = 0.0
		}
	}
	return result, nil
}

// int32ToFloat32 converts an int32 tensor to float32 for GPU operations.
// Used for condition tensors in Where operations.
func int32ToFloat32(condition *tensor.RawTensor) (*tensor.RawTensor, error) {
	result, err := tensor.NewRaw(condition.Shape(), tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}
	intData := condition.AsInt32()
	floatData := result.AsFloat32()
	for i, v := range intData {
		floatData[i] = float32(v)
	}
	return result, nil
}

// runWhere performs conditional element selection on GPU.
// result[i] = condition[i] != 0 ? x[i] : y[i].
// Supports float32 and int32 data types. Condition can be bool, float32, or int32.
//
//nolint:funlen,gocognit,gocyclo,cyclop // Complex GPU operation with dtype handling and broadcasting
func (b *Backend) runWhere(condition, x, y *tensor.RawTensor) (*tensor.RawTensor, error) {
	// Convert condition to float32 for GPU
	var condFloat32 *tensor.RawTensor
	var err error
	switch condition.DType() {
	case tensor.Bool:
		condFloat32, err = boolToFloat32(condition)
		if err != nil {
			return nil, err
		}
	case tensor.Float32:
		condFloat32 = condition
	case tensor.Int32:
		// Convert int32 condition to float32
		condFloat32, err = int32ToFloat32(condition)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("webgpu: Where condition must be bool, float32, or int32, got %s", condition.DType())
	}

	// x and y must have same dtype
	if x.DType() != y.DType() {
		return nil, fmt.Errorf("webgpu: Where requires x and y with same dtype, got %s and %s", x.DType(), y.DType())
	}

	// Only float32 and int32 supported
	dtype := x.DType()
	if dtype != tensor.Float32 && dtype != tensor.Int32 {
		return nil, fmt.Errorf("webgpu: Where supports float32 and int32, got %s", dtype)
	}

	// Handle broadcasting - compute output shape from all 3 tensors (like Burn)
	outShape := condFloat32.Shape()

	// Broadcast condition with x
	if !condFloat32.Shape().Equal(x.Shape()) {
		broadcastedShape, ok, _ := tensor.BroadcastShapes(condFloat32.Shape(), x.Shape())
		if !ok {
			return nil, fmt.Errorf("webgpu: Where condition and x shapes not broadcastable: %v vs %v", condFloat32.Shape(), x.Shape())
		}
		outShape = broadcastedShape
	}

	// Broadcast outShape with y
	if !outShape.Equal(y.Shape()) {
		broadcastedShape, ok, _ := tensor.BroadcastShapes(outShape, y.Shape())
		if !ok {
			return nil, fmt.Errorf("webgpu: Where output and y shapes not broadcastable: %v vs %v", outShape, y.Shape())
		}
		outShape = broadcastedShape
	}

	// Expand all tensors to output shape
	if !condFloat32.Shape().Equal(outShape) {
		condFloat32 = b.Expand(condFloat32, outShape)
	}
	if !x.Shape().Equal(outShape) {
		x = b.Expand(x, outShape)
	}
	if !y.Shape().Equal(outShape) {
		y = b.Expand(y, outShape)
	}

	numElements := condFloat32.NumElements()

	// Select shader based on dtype
	var shaderName, shaderCode string
	if dtype == tensor.Int32 {
		shaderName = "whereInt32"
		shaderCode = whereShaderInt32
	} else {
		shaderName = "where"
		shaderCode = whereShader
	}

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglWhere)

	bufferCondition := b.createBuffer(condFloat32.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferCondition.Release()

	bufferX := b.createBuffer(x.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferX.Release()

	bufferY := b.createBuffer(y.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferY.Release()

	resultSizeWhere := uint64(x.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResultWhere, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSizeWhere,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResultWhere.Release()

	paramsWhere := make([]byte, 16)
	binary.LittleEndian.PutUint32(paramsWhere[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParamsWhere := b.createUniformBuffer(paramsWhere)
	defer bufferParamsWhere.Release()

	condSizeWhere := uint64(condFloat32.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bgWhere := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferCondition, condSizeWhere),
		bufBinding(bufferX, resultSizeWhere),
		bufBinding(bufferY, resultSizeWhere),
		bufBinding(bufferResultWhere, resultSizeWhere),
		bufBinding(bufferParamsWhere, 16),
	})
	defer bgWhere.Release()

	workgroupsWhere := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	resultDataWhere := b.execComputeAndRead(entry.pipeline, bgWhere, workgroupsWhere, 1, 1, bufferResultWhere, resultSizeWhere)

	resultWhere, err := tensor.NewRaw(x.Shape(), dtype, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(resultWhere.Data(), resultDataWhere)
	return resultWhere, nil
}

// runGather gathers elements along a dimension using indices.
// For dim=-1 (last dimension): input[..., indices[...]] -> result[...].
// input: float32 tensor, indices: int32 tensor (like PyTorch/NumPy).
func (b *Backend) runGather(input *tensor.RawTensor, dim int, indices *tensor.RawTensor) (*tensor.RawTensor, error) {
	if input.DType() != tensor.Float32 {
		return nil, fmt.Errorf("webgpu: Gather input must be float32, got %s", input.DType())
	}
	if indices.DType() != tensor.Int32 {
		return nil, fmt.Errorf("webgpu: Gather indices must be int32, got %s", indices.DType())
	}

	inShape := input.Shape()
	idxShape := indices.Shape()
	ndim := len(inShape)

	// Normalize dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// For non-last dimensions: transpose → gather → transpose back
	if dim != ndim-1 {
		return b.gatherNonLastDim(input, dim, indices)
	}

	// Calculate batch size (product of all dimensions except last)
	gatherBatchSize := 1
	for i := 0; i < ndim-1; i++ {
		gatherBatchSize *= inShape[i]
	}
	inputDim := inShape[ndim-1]

	// Output K is the size of the last dimension of indices
	outputK := idxShape[len(idxShape)-1]

	// Result shape: batch dimensions + outputK
	gatherResultShape := make(tensor.Shape, ndim)
	copy(gatherResultShape, inShape[:ndim-1])
	gatherResultShape[ndim-1] = outputK

	shaderGather := b.compileShader("gather", gatherShader)
	entryGather := b.getOrCreatePipeline("gather", shaderGather, bglBinary)

	bufferInputGather := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInputGather.Release()

	bufferIndices := b.createBuffer(indices.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferIndices.Release()

	gatherResultSize := uint64(gatherBatchSize) * uint64(outputK) * 4 //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResultGather, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  gatherResultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResultGather.Release()

	paramsGather := make([]byte, 16)
	binary.LittleEndian.PutUint32(paramsGather[0:4], uint32(gatherBatchSize))
	binary.LittleEndian.PutUint32(paramsGather[4:8], uint32(inputDim)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	binary.LittleEndian.PutUint32(paramsGather[8:12], uint32(outputK)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParamsGather := b.createUniformBuffer(paramsGather)
	defer bufferParamsGather.Release()

	inputGatherSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	indicesSize := uint64(indices.ByteSize())   //nolint:gosec // G115: integer overflow conversion int -> uint64
	bgGather := b.createBindGroupFromBuffers(entryGather.layout, []bindGroupBuffer{
		bufBinding(bufferInputGather, inputGatherSize),
		bufBinding(bufferIndices, indicesSize),
		bufBinding(bufferResultGather, gatherResultSize),
		bufBinding(bufferParamsGather, 16),
	})
	defer bgGather.Release()

	totalOutputGather := gatherBatchSize * outputK
	workgroupsGather := uint32((totalOutputGather + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	resultDataGather := b.execComputeAndRead(entryGather.pipeline, bgGather, workgroupsGather, 1, 1, bufferResultGather, gatherResultSize)

	resultGather, err := tensor.NewRaw(gatherResultShape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(resultGather.Data(), resultDataGather)
	return resultGather, nil
}

// gatherNonLastDim handles Gather on non-last dimensions via transpose.
// Approach: transpose input & indices → gather on last dim → transpose back.
func (b *Backend) gatherNonLastDim(input *tensor.RawTensor, dim int, indices *tensor.RawTensor) (*tensor.RawTensor, error) {
	inShape := input.Shape()
	ndim := len(inShape)

	// Build transpose axes: move dim to last position
	// e.g., for dim=1, ndim=3: [0, 2, 1] (swap 1 and 2)
	axes := make([]int, ndim)
	for i := 0; i < ndim; i++ {
		axes[i] = i
	}
	axes[dim] = ndim - 1
	axes[ndim-1] = dim

	// Transpose input: move target dim to last
	transposedInput := b.Transpose(input, axes...)

	// Transpose indices with same axes (indices must match input dimensions)
	transposedIndices := b.transposeInt32(indices, axes)

	// Gather on last dimension
	gathered, err := b.runGather(transposedInput, -1, transposedIndices)
	if err != nil {
		return nil, err
	}

	// Transpose back
	result := b.Transpose(gathered, axes...) // Same axes work for inverse (swap is symmetric)
	return result, nil
}

// transposeInt32 transposes an int32 tensor.
func (b *Backend) transposeInt32(t *tensor.RawTensor, axes []int) *tensor.RawTensor {
	shape := t.Shape()
	ndim := len(shape)

	// Compute new shape
	newShape := make(tensor.Shape, ndim)
	for i, ax := range axes {
		newShape[i] = shape[ax]
	}

	// Create result
	result, err := tensor.NewRaw(newShape, tensor.Int32, tensor.WebGPU)
	if err != nil {
		panic("webgpu: transposeInt32: " + err.Error())
	}

	// Transpose on CPU
	srcData := t.AsInt32()
	dstData := result.AsInt32()
	srcStrides := shape.ComputeStrides()
	dstStrides := newShape.ComputeStrides()
	numElements := shape.NumElements()

	for i := 0; i < numElements; i++ {
		// Convert flat index to dst coordinates
		dstIdx := i
		coords := make([]int, ndim)
		for d := 0; d < ndim; d++ {
			coords[d] = dstIdx / dstStrides[d]
			dstIdx %= dstStrides[d]
		}

		// Map to src index
		srcIdx := 0
		for d := 0; d < ndim; d++ {
			srcIdx += coords[d] * srcStrides[axes[d]]
		}

		dstData[i] = srcData[srcIdx]
	}

	return result
}

// runTransposeND executes N-dimensional matrix transpose on GPU.
// Supports up to 6D tensors with arbitrary axes permutation.
//
//nolint:gocognit,gocyclo,cyclop,funlen // Complex GPU setup logic - unavoidable for parameter packing
func (b *Backend) runTransposeND(input *tensor.RawTensor, axes []int) (*tensor.RawTensor, error) {
	shape := input.Shape()
	ndim := len(shape)

	if ndim > 6 {
		return nil, fmt.Errorf("webgpu: transposeND supports up to 6D tensors, got %dD", ndim)
	}

	// Default axes: reverse all dimensions
	if len(axes) == 0 {
		axes = make([]int, ndim)
		for i := 0; i < ndim; i++ {
			axes[i] = ndim - 1 - i
		}
	}

	if len(axes) != ndim {
		return nil, fmt.Errorf("webgpu: transpose axes length must match tensor dimensions")
	}

	// Validate axes
	seen := make(map[int]bool)
	for _, ax := range axes {
		if ax < 0 || ax >= ndim {
			return nil, fmt.Errorf("webgpu: axis %d out of range for %dD tensor", ax, ndim)
		}
		if seen[ax] {
			return nil, fmt.Errorf("webgpu: duplicate axis %d", ax)
		}
		seen[ax] = true
	}

	// Compute new shape
	newShape := make(tensor.Shape, ndim)
	for i, ax := range axes {
		newShape[i] = shape[ax]
	}

	// Choose shader based on dtype
	var shaderName, shaderCode string
	switch input.DType() {
	case tensor.Float32:
		shaderName = "transposeND"
		shaderCode = transposeNDShader
	case tensor.Int32:
		shaderName = "transposeND_int32"
		shaderCode = transposeNDShaderInt32
	default:
		return nil, fmt.Errorf("webgpu: transposeND unsupported dtype %s", input.DType())
	}

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	// Uniform layout: ndim, total_elements, shape[6], input_strides[6], output_strides[6], axes[6]
	// = 26 u32 values × 4 bytes = 104 bytes.
	params := make([]byte, 4*26)
	inputStrides := shape.ComputeStrides()
	outputStrides := newShape.ComputeStrides()

	binary.LittleEndian.PutUint32(params[0:4], uint32(ndim))
	binary.LittleEndian.PutUint32(params[4:8], uint32(shape.NumElements())) //nolint:gosec // G115: integer overflow conversion int -> uint32

	for i := 0; i < 6; i++ {
		if i < len(shape) {
			binary.LittleEndian.PutUint32(params[8+i*4:12+i*4], uint32(shape[i])) //nolint:gosec // G115: safe, shape values are non-negative and fit in uint32
		} else {
			binary.LittleEndian.PutUint32(params[8+i*4:12+i*4], 1)
		}
	}
	for i := 0; i < 6; i++ {
		if i < len(inputStrides) {
			binary.LittleEndian.PutUint32(params[32+i*4:36+i*4], uint32(inputStrides[i])) //nolint:gosec // G115: safe, stride values are non-negative and fit in uint32
		} else {
			binary.LittleEndian.PutUint32(params[32+i*4:36+i*4], 1)
		}
	}
	for i := 0; i < 6; i++ {
		if i < len(outputStrides) {
			binary.LittleEndian.PutUint32(params[56+i*4:60+i*4], uint32(outputStrides[i])) //nolint:gosec // G115: safe, stride values are non-negative and fit in uint32
		} else {
			binary.LittleEndian.PutUint32(params[56+i*4:60+i*4], 1)
		}
	}
	for i := 0; i < 6; i++ {
		if i < len(axes) {
			binary.LittleEndian.PutUint32(params[80+i*4:84+i*4], uint32(axes[i])) //nolint:gosec // G115: safe, axis indices are non-negative and fit in uint32
		} else {
			binary.LittleEndian.PutUint32(params[80+i*4:84+i*4], 0)
		}
	}

	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	paramsSize := uint64(len(params))
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, paramsSize),
	})
	defer bg.Release()

	// 1D workgroups, 256 threads each.
	numElements := uint32(shape.NumElements()) //nolint:gosec // G115: integer overflow conversion int -> uint32
	workgroups := uint32(math.Ceil(float64(numElements) / 256.0))
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(newShape, input.DType(), tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}

// runExpand executes NumPy-style broadcasting on GPU.
// Expands tensor to new shape by broadcasting dimensions of size 1.
//
//nolint:gocognit,gocyclo,cyclop,funlen // Complex GPU setup logic - unavoidable for parameter packing
func (b *Backend) runExpand(input *tensor.RawTensor, newShape tensor.Shape) (*tensor.RawTensor, error) {
	shape := input.Shape()

	// Validate shapes are compatible for broadcasting
	if len(newShape) < len(shape) {
		return nil, fmt.Errorf("webgpu: expand new shape must have at least as many dimensions")
	}

	if len(newShape) > 6 {
		return nil, fmt.Errorf("webgpu: expand supports up to 6D tensors, got %dD", len(newShape))
	}

	// Pad source shape to match destination dimensions
	dimDiff := len(newShape) - len(shape)
	paddedShape := make(tensor.Shape, len(newShape))
	for i := 0; i < dimDiff; i++ {
		paddedShape[i] = 1
	}
	for i := 0; i < len(shape); i++ {
		paddedShape[dimDiff+i] = shape[i]
	}

	// Validate broadcasting compatibility
	for i := 0; i < len(newShape); i++ {
		if paddedShape[i] != 1 && paddedShape[i] != newShape[i] {
			return nil, fmt.Errorf("webgpu: expand incompatible shapes: %v -> %v", shape, newShape)
		}
	}

	// Choose shader based on dtype
	var shaderName, shaderCode string
	switch input.DType() {
	case tensor.Float32:
		shaderName = "expand"
		shaderCode = expandShader
	case tensor.Int32:
		shaderName = "expand_int32"
		shaderCode = expandShaderInt32
	default:
		return nil, fmt.Errorf("webgpu: expand unsupported dtype %s", input.DType())
	}

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	bufferInput := b.createBuffer(input.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	defer bufferInput.Release()

	resultNumElements := newShape.NumElements()
	elementSize := uint64(input.DType().Size())           //nolint:gosec // G115: integer overflow conversion int -> uint64
	resultSize := uint64(resultNumElements) * elementSize //nolint:gosec // G115: integer overflow conversion int -> uint64

	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: create result buffer: %w", err)
	}
	defer bufferResult.Release()

	// Uniform layout: ndim, total_elements, input_shape[6], input_strides[6], output_strides[6]
	// = 20 u32 values × 4 bytes = 80 bytes.
	params := make([]byte, 4*20)
	inputStrides := paddedShape.ComputeStrides()
	outputStrides := newShape.ComputeStrides()

	binary.LittleEndian.PutUint32(params[0:4], uint32(len(newShape)))     //nolint:gosec // G115: integer overflow conversion int -> uint32
	binary.LittleEndian.PutUint32(params[4:8], uint32(resultNumElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32

	for i := 0; i < 6; i++ {
		if i < len(paddedShape) {
			binary.LittleEndian.PutUint32(params[8+i*4:12+i*4], uint32(paddedShape[i])) //nolint:gosec // G115: safe, shape values are non-negative and fit in uint32
		} else {
			binary.LittleEndian.PutUint32(params[8+i*4:12+i*4], 1)
		}
	}
	for i := 0; i < 6; i++ {
		if i < len(inputStrides) {
			binary.LittleEndian.PutUint32(params[32+i*4:36+i*4], uint32(inputStrides[i])) //nolint:gosec // G115: safe, stride values are non-negative and fit in uint32
		} else {
			binary.LittleEndian.PutUint32(params[32+i*4:36+i*4], 1)
		}
	}
	for i := 0; i < 6; i++ {
		if i < len(outputStrides) {
			binary.LittleEndian.PutUint32(params[56+i*4:60+i*4], uint32(outputStrides[i])) //nolint:gosec // G115: safe, stride values are non-negative and fit in uint32
		} else {
			binary.LittleEndian.PutUint32(params[56+i*4:60+i*4], 1)
		}
	}

	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	inputSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	paramsSize := uint64(len(params))
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(bufferInput, inputSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, paramsSize),
	})
	defer bg.Release()

	// 1D workgroups, 256 threads each.
	workgroups := uint32(math.Ceil(float64(resultNumElements) / 256.0))
	resultData := b.execComputeAndRead(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize)

	result, err := tensor.NewRaw(newShape, input.DType(), tensor.WebGPU)
	if err != nil {
		return nil, err
	}

	copy(result.Data(), resultData)
	return result, nil
}
