//go:build windows || linux

// Package webgpu implements the WebGPU backend for GPU-accelerated tensor operations.
package webgpu

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"unsafe"

	"github.com/born-ml/born/internal/tensor"
	"github.com/gogpu/gputypes"
	wgpu "github.com/gogpu/wgpu"
)

// getEnvIntOr reads an integer environment variable, returning defaultVal if unset or invalid.
func getEnvIntOr(key string, defaultVal int) int {
	if s := os.Getenv(key); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return v
		}
	}
	return defaultVal
}

// createLazyResult creates a lazy RawTensor backed by the compute result buffer
// (Storage | CopySrc). The resultBuf is kept alive by LazyGPUData until the
// tensor is realized (Data() called) or GC'd.
//
// Ownership of resultBuf is transferred to the lazy tensor:
//   - It is NOT released here — the caller must NOT defer-release it.
//   - It will be released when LazyGPUData.Release() is called (GC or explicit),
//     which invokes ReleaseGPUBuffer on the backend.
//
// When Data() is called on the result tensor, ReadGPUBuffer() creates a
// transient MapRead staging buffer, copies resultBuf into it, maps the staging
// buffer, reads the bytes, then releases the staging buffer — all inline.
func (b *Backend) createLazyResult(resultBuf *wgpu.Buffer, bufferSize uint64, shape tensor.Shape, dtype tensor.DataType) (*tensor.RawTensor, error) {
	// Create lazy GPU data referencing the result (Storage|CopySrc) buffer.
	gpuData := tensor.NewLazyGPUData(unsafe.Pointer(resultBuf), resultBuf, bufferSize, b) //nolint:gosec // G103: Required for GPU buffer tracking

	// Create lazy tensor — CPU buffer allocated but not filled until Data() is called.
	result, err := tensor.NewLazyRaw(shape, dtype, tensor.WebGPU, gpuData)
	if err != nil {
		// If tensor creation fails, release the result buffer.
		resultBuf.Release()
		return nil, err
	}

	return result, nil
}

// runBinaryOpLazy executes a binary element-wise operation and returns a LAZY tensor.
// The result stays on GPU until Data() is called.
func (b *Backend) runBinaryOpLazy(a, other *tensor.RawTensor, shaderName, shaderCode string) (*tensor.RawTensor, error) {
	// Validate inputs - must have same dtype
	if a.DType() != other.DType() {
		return nil, errDTypeMismatch(a.DType(), other.DType())
	}

	// Only float32 and int32 are supported
	dtype := a.DType()
	if dtype != tensor.Float32 && dtype != tensor.Int32 {
		return nil, errUnsupportedDType(dtype)
	}

	// Handle broadcasting if shapes don't match
	if !a.Shape().Equal(other.Shape()) {
		broadcastedShape, ok, _ := tensor.BroadcastShapes(a.Shape(), other.Shape())
		if !ok {
			return nil, errBroadcastFailed(a.Shape(), other.Shape())
		}
		// Expand tensors to broadcasted shape
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

	// Get or create GPU buffers for inputs. Cached CPU tensors (e.g. weight
	// matrices) reuse the same GPU buffer across calls. Lazy (GPU-backed) tensors
	// return their existing result buffer directly (cached:true — no copy needed).
	// Ownership: cached buffers are NOT added to lazyResources; they live for the
	// duration of the Backend (CPU) or the lazy tensor (GPU).
	inputA := b.getOrCreateInputBuffer(a)
	inputOther := b.getOrCreateInputBuffer(other)

	// Collect transient (non-cached) input buffers for release after Submit.
	// Collect gpuData from lazy inputs to keep them alive until Submit.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputA.cached {
		transientBufs = append(transientBufs, inputA.buffer)
	} else if inputA.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputA.gpuData)
	}
	if !inputOther.cached {
		transientBufs = append(transientBufs, inputOther.buffer)
	} else if inputOther.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputOther.gpuData)
	}

	resultSize := uint64(a.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	// Storage|CopySrc allows both shader writes and the readback copy in ReadGPUBuffer.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runBinaryOpLazy: create result buffer: %w", err)
	}

	// Create uniform buffer for params. Ownership transfers to addComputePassToEncoder.
	params := b.createParamsBuffer(numElements)

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputA.buffer, resultSize),
		bufBinding(inputOther.buffer, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(params, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, a.Shape(), a.DType(),
		lazyResources{
			buffers:    append(transientBufs, params),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// lazyResources collects GPU resources that must stay alive until after
// queue.Submit. Ownership transfers to the encoder batch via addComputePassToEncoder
// (see encoder_batch.go). Resources are released in flushCommands after Submit.
//
// buffers holds params buffers and transient input copies (NOT result buffers).
// Result buffers are owned by LazyGPUData and released via ReleaseGPUBuffer.
//
// lazyDatas holds LazyGPUData pointers for lazy input tensors. These are tracked
// in the encoder batch to prevent the GC from running their finalizers (which
// release the underlying result buffers) while the command buffer referencing
// those buffers is still pending submission.
//
// BUG-LAZY-DEFER-RELEASE: res.buffers must remain alive until after
// queue.Submit, because wgpu's validateCommandBufferForSubmit rejects
// command buffers that reference released buffers (released.Load() == true).
type lazyResources struct {
	buffers    []*wgpu.Buffer
	bindGroups []*wgpu.BindGroup
	lazyDatas  []*tensor.LazyGPUData // input lazy tensors kept alive until Submit
}

// maxPendingBeforeFlush limits how many compute passes accumulate in the
// shared encoder before auto-flushing. Serves two purposes:
//   - Prevents Windows TDR timeout (default 2s) on iGPUs
//   - Bounds GPU memory usage (each pass holds result+staging+params buffers)
//
// 64 is a balance between Burn/CubeCL's 32 and Born's original 128:
// enough batching for throughput without TDR risk on integrated GPUs.
// Configurable via BORN_MAX_TASKS environment variable.
var maxPendingBeforeFlush = getEnvIntOr("BORN_MAX_TASKS", 64)

// copyGPUBuffer creates a GPU-to-GPU copy without CPU round-trip.
// This is critical for LazyMode performance - avoids GPU→CPU→GPU transfers.
func (b *Backend) copyGPUBuffer(srcBuffer *wgpu.Buffer, size uint64) *wgpu.Buffer {
	// Flush pending commands first — srcBuffer may be a staging buffer from a
	// lazy op whose command buffer hasn't been submitted yet. Without this
	// flush, CopyBufferToBuffer would read uninitialized staging data.
	// flushCommands calls finishActiveBatchLocked internally, so the active
	// encoder (if any) is also finished before we issue the copy submit.
	b.flushCommands()
	b.device.Poll(wgpu.PollWait)

	dstBuffer, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  size,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: copyGPUBuffer: failed to create dst buffer: %v", err))
	}

	encoder, encErr := b.device.CreateCommandEncoder(nil)
	if encErr != nil {
		panic(fmt.Sprintf("webgpu: copyGPUBuffer: failed to create encoder: %v", encErr))
	}
	encoder.CopyBufferToBuffer(srcBuffer, 0, dstBuffer, 0, size)
	cmdBuffer, finErr := encoder.Finish()
	if finErr != nil {
		panic(fmt.Sprintf("webgpu: copyGPUBuffer: failed to finish encoder: %v", finErr))
	}
	if _, err := b.queue.Submit(cmdBuffer); err != nil {
		panic(fmt.Sprintf("webgpu: copyGPUBuffer: submit failed: %v", err))
	}

	return dstBuffer
}

// createBufferFromTensor creates a GPU buffer from a RawTensor.
// If the tensor already has GPU data (lazy), performs GPU→GPU copy (no CPU round-trip!).
func (b *Backend) createBufferFromTensor(t *tensor.RawTensor) *wgpu.Buffer {
	// Check if tensor already has GPU data
	if gpuData := t.GPUData(); gpuData != nil && !gpuData.IsRealized() {
		// Tensor has unrealized GPU data - use GPU→GPU copy.
		// KeepAlive prevents GC from collecting gpuData (and running its
		// finalizer which releases the buffer) while copyGPUBuffer uses it.
		existingBuffer := (*wgpu.Buffer)(gpuData.BufferPtr())
		result := b.copyGPUBuffer(existingBuffer, gpuData.Size())
		runtime.KeepAlive(gpuData)
		return result
	}

	// CPU tensor - upload data to GPU
	return b.createBuffer(t.Data(), gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
}

// createParamsBuffer creates a uniform buffer with element count parameter.
func (b *Backend) createParamsBuffer(numElements int) *wgpu.Buffer {
	params := make([]byte, 16)                    // 16-byte aligned
	putUint32LE(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.createUniformBuffer(params)
}

// errDTypeMismatch returns an error for dtype mismatch.
func errDTypeMismatch(a, other tensor.DataType) error {
	return &lazyError{msg: "dtype mismatch: " + a.String() + " vs " + other.String()}
}

func errUnsupportedDType(dtype tensor.DataType) error {
	return &lazyError{msg: "unsupported dtype: " + dtype.String() + " (only float32 and int32)"}
}

func errBroadcastFailed(_, _ tensor.Shape) error {
	return &lazyError{msg: "shapes not broadcastable"}
}

type lazyError struct {
	msg string
}

func (e *lazyError) Error() string {
	return "webgpu: " + e.msg
}

// putUint32LE writes a uint32 to a byte slice in little-endian order.
func putUint32LE(b []byte, v uint32) {
	b[0] = byte(v)       //nolint:gosec // G115: intentional uint32-to-byte truncation for LE encoding
	b[1] = byte(v >> 8)  //nolint:gosec // G115: intentional uint32-to-byte truncation for LE encoding
	b[2] = byte(v >> 16) //nolint:gosec // G115: intentional uint32-to-byte truncation for LE encoding
	b[3] = byte(v >> 24)
}

// =============================================================================
// Extended Lazy Operations (Phase 3.1)
// =============================================================================

// runMatMulLazy executes matrix multiplication C = A @ B on GPU with lazy result.
// A is [M, K], B is [K, N], C is [M, N].
func (b *Backend) runMatMulLazy(a, other *tensor.RawTensor) (*tensor.RawTensor, error) {
	// Validate inputs
	if a.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "matmul: only float32 is supported, got " + a.DType().String()}
	}
	if len(a.Shape()) != 2 || len(other.Shape()) != 2 {
		return nil, &lazyError{msg: "matmul: requires 2D tensors"}
	}

	M := uint32(a.Shape()[0])     //nolint:gosec // G115: safe, tensor dims are small positive ints
	K := uint32(a.Shape()[1])     //nolint:gosec // G115: safe, tensor dims are small positive ints
	N := uint32(other.Shape()[1]) //nolint:gosec // G115: safe, tensor dims are small positive ints

	if other.Shape()[0] != int(K) {
		return nil, &lazyError{msg: "matmul: shape mismatch"}
	}

	shader := b.compileShader("matmul", matmulShader)
	entry := b.getOrCreatePipeline("matmul", shader, bglBinary)

	// Get or create GPU buffers for inputs. Cached CPU tensors reuse the same
	// GPU buffer. Lazy GPU tensors return their result buffer directly (no copy).
	inputA := b.getOrCreateInputBuffer(a)
	inputOther := b.getOrCreateInputBuffer(other)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputA.cached {
		transientBufs = append(transientBufs, inputA.buffer)
	} else if inputA.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputA.gpuData)
	}
	if !inputOther.cached {
		transientBufs = append(transientBufs, inputOther.buffer)
	} else if inputOther.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputOther.gpuData)
	}

	resultShape := tensor.Shape{int(M), int(N)}
	resultSize := uint64(int(M) * int(N) * 4) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runMatMulLazy: create result buffer: %w", err)
	}

	// Create params buffer. Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	putUint32LE(params[0:4], M)
	putUint32LE(params[4:8], K)
	putUint32LE(params[8:12], N)
	bufferParams := b.createUniformBuffer(params)

	sizeA := uint64(a.ByteSize())         //nolint:gosec // G115: integer overflow conversion int -> uint64
	sizeOther := uint64(other.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputA.buffer, sizeA),
		bufBinding(inputOther.buffer, sizeOther),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	// 2D workgroups (16x16 per workgroup)
	workgroupsX := (N + 15) / 16
	workgroupsY := (M + 15) / 16
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroupsX, workgroupsY, 1, bufferResult, resultSize, resultShape, tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runUnaryOpLazy executes a unary operation (exp, sqrt, cos, sin, etc.) with lazy result.
func (b *Backend) runUnaryOpLazy(x *tensor.RawTensor, shaderName, shaderCode string) (*tensor.RawTensor, error) {
	if x.DType() != tensor.Float32 {
		return nil, &lazyError{msg: shaderName + ": only float32 is supported"}
	}

	numElements := x.NumElements()
	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	// Get or create GPU buffer for input. Ownership rules: cached buffers are
	// not added to lazyResources; non-cached GPU copies must be released after Submit.
	inputX := b.getOrCreateInputBuffer(x)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputX.cached {
		transientBufs = append(transientBufs, inputX.buffer)
	} else if inputX.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputX.gpuData)
	}

	resultSize := uint64(x.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runUnaryOpLazy: create result buffer: %w", err)
	}

	// Create params buffer. Ownership transfers to addComputePassToEncoder.
	params := b.createParamsBuffer(numElements)

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputX.buffer, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(params, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, x.Shape(), tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, params),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runScalarOpLazy executes a scalar operation (mul, add, sub, div by scalar) with lazy result.
func (b *Backend) runScalarOpLazy(x *tensor.RawTensor, scalar float32, shaderName, shaderCode string) (*tensor.RawTensor, error) {
	if x.DType() != tensor.Float32 {
		return nil, &lazyError{msg: shaderName + ": only float32 is supported"}
	}

	numElements := x.NumElements()
	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	// Get or create GPU buffer for input. Cached CPU tensors reuse the same buffer.
	inputX := b.getOrCreateInputBuffer(x)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputX.cached {
		transientBufs = append(transientBufs, inputX.buffer)
	} else if inputX.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputX.gpuData)
	}

	resultSize := uint64(x.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runScalarOpLazy: create result buffer: %w", err)
	}

	// Create params buffer with scalar value. Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	putUint32LE(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	putFloat32LE(params[4:8], scalar)
	bufferParams := b.createUniformBuffer(params)

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputX.buffer, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, x.Shape(), tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// putFloat32LE writes a float32 to a byte slice in little-endian order.
func putFloat32LE(b []byte, v float32) {
	bits := *(*uint32)(unsafe.Pointer(&v)) //nolint:gosec // G103: Required for float bit conversion
	putUint32LE(b, bits)
}

// runBatchMatMulLazy executes batched matrix multiplication on GPU with lazy result.
// Supports 3D [batch, M, K] @ [batch, K, N] and 4D [batch, heads, M, K] @ [batch, heads, K, N].
func (b *Backend) runBatchMatMulLazy(a, other *tensor.RawTensor) (*tensor.RawTensor, error) {
	// Validate inputs
	if a.DType() != tensor.Float32 || other.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "batchMatMul: only float32 is supported"}
	}

	shapeA := a.Shape()
	shapeB := other.Shape()

	if len(shapeA) != len(shapeB) || (len(shapeA) != 3 && len(shapeA) != 4) {
		return nil, &lazyError{msg: "batchMatMul: requires 3D or 4D tensors with matching dimensions"}
	}

	var batch, M, K, N uint32
	var resultShape tensor.Shape

	if len(shapeA) == 3 {
		// 3D: [batch, M, K] @ [batch, K, N]
		batch = uint32(shapeA[0]) //nolint:gosec // G115: safe, tensor dims are small positive ints
		M = uint32(shapeA[1])     //nolint:gosec // G115: safe, tensor dims are small positive ints
		K = uint32(shapeA[2])     //nolint:gosec // G115: safe, tensor dims are small positive ints
		N = uint32(shapeB[2])     //nolint:gosec // G115: safe, tensor dims are small positive ints
		resultShape = tensor.Shape{int(batch), int(M), int(N)}
	} else {
		// 4D: [batch, heads, M, K] @ [batch, heads, K, N]
		batch = uint32(shapeA[0] * shapeA[1]) //nolint:gosec // G115: safe, product of small tensor dims
		M = uint32(shapeA[2])                 //nolint:gosec // G115: safe, tensor dims are small positive ints
		K = uint32(shapeA[3])                 //nolint:gosec // G115: safe, tensor dims are small positive ints
		N = uint32(shapeB[3])                 //nolint:gosec // G115: safe, tensor dims are small positive ints
		resultShape = tensor.Shape{shapeA[0], shapeA[1], int(M), int(N)}
	}

	shader := b.compileShader("batchMatMul", batchMatMulShader)
	entry := b.getOrCreatePipeline("batchMatMul", shader, bglBinary)

	// Get or create GPU buffers for inputs. Cached CPU tensors reuse the same buffer.
	inputA := b.getOrCreateInputBuffer(a)
	inputB := b.getOrCreateInputBuffer(other)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputA.cached {
		transientBufs = append(transientBufs, inputA.buffer)
	} else if inputA.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputA.gpuData)
	}
	if !inputB.cached {
		transientBufs = append(transientBufs, inputB.buffer)
	} else if inputB.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputB.gpuData)
	}

	resultSize := uint64(batch) * uint64(M) * uint64(N) * 4 // float32 = 4 bytes

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runBatchMatMulLazy: create result buffer: %w", err)
	}

	// Create uniform buffer for params. Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	putUint32LE(params[0:4], batch)
	putUint32LE(params[4:8], M)
	putUint32LE(params[8:12], K)
	putUint32LE(params[12:16], N)
	bufferParams := b.createUniformBuffer(params)

	sizeA := uint64(a.ByteSize())     //nolint:gosec // G115: integer overflow conversion int -> uint64
	sizeB := uint64(other.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputA.buffer, sizeA),
		bufBinding(inputB.buffer, sizeB),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	// Dispatch: (N+7)/8 x (M+7)/8 x batch
	workgroupsX := (N + 7) / 8
	workgroupsY := (M + 7) / 8
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroupsX, workgroupsY, batch, bufferResult, resultSize, resultShape, tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runTransposeLazy executes 2D matrix transpose with lazy result.
func (b *Backend) runTransposeLazy(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	if input.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "transpose: only float32 is supported"}
	}
	if len(input.Shape()) != 2 {
		return nil, &lazyError{msg: "transpose: requires 2D tensor"}
	}

	rows := uint32(input.Shape()[0]) //nolint:gosec // G115: safe, tensor dims are small positive ints
	cols := uint32(input.Shape()[1]) //nolint:gosec // G115: safe, tensor dims are small positive ints

	shader := b.compileShader("transpose", transposeShader)
	entry := b.getOrCreatePipeline("transpose", shader, bglUnary)

	// Get or create GPU buffer for input. Cached CPU tensors reuse the same buffer.
	inputResult := b.getOrCreateInputBuffer(input)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputResult.cached {
		transientBufs = append(transientBufs, inputResult.buffer)
	} else if inputResult.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputResult.gpuData)
	}

	resultShape := tensor.Shape{int(cols), int(rows)}
	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runTransposeLazy: create result buffer: %w", err)
	}

	// Create params buffer. Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	putUint32LE(params[0:4], rows)
	putUint32LE(params[4:8], cols)
	bufferParams := b.createUniformBuffer(params)

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputResult.buffer, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	workgroupsX := (cols + 15) / 16
	workgroupsY := (rows + 15) / 16
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroupsX, workgroupsY, 1, bufferResult, resultSize, resultShape, tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runSoftmaxLazy executes softmax on GPU with lazy result.
// Input must be 2D [batch, classes].
func (b *Backend) runSoftmaxLazy(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	// Validate input
	if input.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "softmax: only float32 is supported"}
	}
	if len(input.Shape()) != 2 {
		return nil, &lazyError{msg: "softmax: requires 2D tensor"}
	}

	batchSize := uint32(input.Shape()[0])  //nolint:gosec // G115: safe, tensor dims are small positive ints
	numClasses := uint32(input.Shape()[1]) //nolint:gosec // G115: safe, tensor dims are small positive ints

	shader := b.compileShader("softmax", softmaxShader)
	entry := b.getOrCreatePipeline("softmax", shader, bglUnary)

	// Get or create GPU buffer for input. Cached CPU tensors reuse the same buffer.
	inputResult := b.getOrCreateInputBuffer(input)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputResult.cached {
		transientBufs = append(transientBufs, inputResult.buffer)
	} else if inputResult.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputResult.gpuData)
	}

	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runSoftmaxLazy: create result buffer: %w", err)
	}

	// Create uniform buffer for params. Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	putUint32LE(params[0:4], batchSize)
	putUint32LE(params[4:8], numClasses)
	bufferParams := b.createUniformBuffer(params)

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputResult.buffer, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	// Each workgroup handles one row (batch sample).
	workgroups := (batchSize + workgroupSize - 1) / workgroupSize
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, input.Shape(), tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runTransposeNDLazy executes N-dimensional transpose on GPU with lazy result.
// Supports up to 6D tensors with arbitrary axes permutation.
//
//nolint:gocognit,gocyclo,cyclop,funlen // Complex GPU setup logic - unavoidable for parameter packing
func (b *Backend) runTransposeNDLazy(input *tensor.RawTensor, axes []int) (*tensor.RawTensor, error) {
	shape := input.Shape()
	ndim := len(shape)

	if ndim > 6 {
		return nil, &lazyError{msg: "transposeND: supports up to 6D tensors"}
	}

	// Default axes: reverse all dimensions
	if len(axes) == 0 {
		axes = make([]int, ndim)
		for i := 0; i < ndim; i++ {
			axes[i] = ndim - 1 - i
		}
	}

	if len(axes) != ndim {
		return nil, &lazyError{msg: "transposeND: axes length must match tensor dimensions"}
	}

	// Validate axes
	seen := make(map[int]bool)
	for _, ax := range axes {
		if ax < 0 || ax >= ndim {
			return nil, &lazyError{msg: "transposeND: axis out of range"}
		}
		if seen[ax] {
			return nil, &lazyError{msg: "transposeND: duplicate axis"}
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
		return nil, &lazyError{msg: "transposeND: unsupported dtype " + input.DType().String()}
	}

	// Compile shader and get pipeline
	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	// Get or create GPU buffer for input. Cached CPU tensors reuse the same buffer.
	inputResult := b.getOrCreateInputBuffer(input)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputResult.cached {
		transientBufs = append(transientBufs, inputResult.buffer)
	} else if inputResult.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputResult.gpuData)
	}

	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, bufErr := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
		Size:  resultSize,
	})
	if bufErr != nil {
		return nil, fmt.Errorf("runTransposeNDLazy: create result buffer: %w", bufErr)
	}

	// Create uniform buffer for params. Ownership transfers to addComputePassToEncoder.
	// Layout: ndim, total_elements, shapes[6], input_strides[6], output_strides[6], axes[6]
	params := make([]byte, 4*26) // 26 u32 values * 4 bytes
	inputStrides := shape.ComputeStrides()
	outputStrides := newShape.ComputeStrides()

	putUint32LE(params[0:4], uint32(ndim))
	putUint32LE(params[4:8], uint32(shape.NumElements())) //nolint:gosec // G115: integer overflow conversion int -> uint32

	// Pack input shape (6 slots)
	for i := 0; i < 6; i++ {
		if i < len(shape) {
			putUint32LE(params[8+i*4:12+i*4], uint32(shape[i])) //nolint:gosec // G115: safe, tensor dims are small positive ints
		} else {
			putUint32LE(params[8+i*4:12+i*4], 1)
		}
	}

	// Pack input strides (6 slots)
	for i := 0; i < 6; i++ {
		if i < len(inputStrides) {
			putUint32LE(params[32+i*4:36+i*4], uint32(inputStrides[i])) //nolint:gosec // G115: safe, strides derived from tensor dims
		} else {
			putUint32LE(params[32+i*4:36+i*4], 1)
		}
	}

	// Pack output strides (6 slots)
	for i := 0; i < 6; i++ {
		if i < len(outputStrides) {
			putUint32LE(params[56+i*4:60+i*4], uint32(outputStrides[i])) //nolint:gosec // G115: safe, strides derived from tensor dims
		} else {
			putUint32LE(params[56+i*4:60+i*4], 1)
		}
	}

	// Pack axes (6 slots)
	for i := 0; i < 6; i++ {
		if i < len(axes) {
			putUint32LE(params[80+i*4:84+i*4], uint32(axes[i])) //nolint:gosec // G115: safe, axis indices are small non-negative ints
		} else {
			putUint32LE(params[80+i*4:84+i*4], 0)
		}
	}

	bufferParams := b.createUniformBuffer(params)

	paramsSize := uint64(len(params))
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputResult.buffer, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, paramsSize),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	// Calculate workgroup count (1D workgroups, 256 threads each).
	numElements := uint32(shape.NumElements()) //nolint:gosec // G115: integer overflow conversion int -> uint32
	workgroups := (numElements + 255) / 256
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, newShape, input.DType(),
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runExpandLazy broadcasts tensor to new shape with lazy result.
// Supports up to 6D tensors.
//
//nolint:gocognit,gocyclo,cyclop // Complex GPU setup logic - unavoidable for parameter packing
func (b *Backend) runExpandLazy(input *tensor.RawTensor, newShape tensor.Shape) (*tensor.RawTensor, error) {
	shape := input.Shape()

	// Validate shapes are compatible for broadcasting
	if len(newShape) < len(shape) {
		return nil, &lazyError{msg: "expand: new shape must have at least as many dimensions"}
	}

	if len(newShape) > 6 {
		return nil, &lazyError{msg: "expand: supports up to 6D tensors"}
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
			return nil, &lazyError{msg: "expand: incompatible shapes"}
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
		return nil, &lazyError{msg: "expand: unsupported dtype " + input.DType().String()}
	}

	// Compile shader and get pipeline
	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	// Get or create GPU buffer for input. Cached CPU tensors reuse the same buffer.
	inputResult := b.getOrCreateInputBuffer(input)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputResult.cached {
		transientBufs = append(transientBufs, inputResult.buffer)
	} else if inputResult.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputResult.gpuData)
	}

	// Calculate result size
	resultNumElements := newShape.NumElements()
	elementSize := uint64(input.DType().Size())           //nolint:gosec // G115: integer overflow conversion int -> uint64
	resultSize := uint64(resultNumElements) * elementSize //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, bufErr := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
		Size:  resultSize,
	})
	if bufErr != nil {
		return nil, fmt.Errorf("runExpandLazy: create result buffer: %w", bufErr)
	}

	// Create uniform buffer for params. Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 4*20) // 20 u32 values * 4 bytes
	inputStrides := paddedShape.ComputeStrides()
	outputStrides := newShape.ComputeStrides()

	putUint32LE(params[0:4], uint32(len(newShape)))     //nolint:gosec // G115: integer overflow conversion int -> uint32
	putUint32LE(params[4:8], uint32(resultNumElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32

	// Pack input shape (6 slots) - use paddedShape
	for i := 0; i < 6; i++ {
		if i < len(paddedShape) {
			putUint32LE(params[8+i*4:12+i*4], uint32(paddedShape[i])) //nolint:gosec // G115: safe, tensor dims are small positive ints
		} else {
			putUint32LE(params[8+i*4:12+i*4], 1)
		}
	}

	// Pack input strides (6 slots)
	for i := 0; i < 6; i++ {
		if i < len(inputStrides) {
			putUint32LE(params[32+i*4:36+i*4], uint32(inputStrides[i])) //nolint:gosec // G115: safe, strides derived from tensor dims
		} else {
			putUint32LE(params[32+i*4:36+i*4], 1)
		}
	}

	// Pack output strides (6 slots)
	for i := 0; i < 6; i++ {
		if i < len(outputStrides) {
			putUint32LE(params[56+i*4:60+i*4], uint32(outputStrides[i])) //nolint:gosec // G115: safe, strides derived from tensor dims
		} else {
			putUint32LE(params[56+i*4:60+i*4], 1)
		}
	}

	bufferParams := b.createUniformBuffer(params)

	inputSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	paramsSize := uint64(len(params))
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputResult.buffer, inputSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, paramsSize),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	workgroups := uint32((resultNumElements + 255) / 256) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, newShape, input.DType(),
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runGatherLazy executes Gather operation with lazy result.
// Input must be float32, indices must be int32.
func (b *Backend) runGatherLazy(input *tensor.RawTensor, dim int, indices *tensor.RawTensor) (*tensor.RawTensor, error) {
	if input.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "gather: input must be float32"}
	}
	if indices.DType() != tensor.Int32 {
		return nil, &lazyError{msg: "gather: indices must be int32"}
	}

	inShape := input.Shape()
	idxShape := indices.Shape()
	ndim := len(inShape)

	// Normalize dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// For non-last dimensions: use non-lazy path (involves multiple operations)
	if dim != ndim-1 {
		// Fall back to non-lazy for complex transpose chain
		return b.gatherNonLastDim(input, dim, indices)
	}

	// Calculate batch size
	gatherBatchSize := 1
	for i := 0; i < ndim-1; i++ {
		gatherBatchSize *= inShape[i]
	}
	inputDim := inShape[ndim-1]
	outputK := idxShape[len(idxShape)-1]

	// Result shape
	gatherResultShape := make(tensor.Shape, ndim)
	copy(gatherResultShape, inShape[:ndim-1])
	gatherResultShape[ndim-1] = outputK

	shader := b.compileShader("gather", gatherShader)
	entry := b.getOrCreatePipeline("gather", shader, bglBinary)

	// Get or create GPU buffers for inputs. Cached CPU tensors reuse the same buffer.
	inputResult := b.getOrCreateInputBuffer(input)
	indicesResult := b.getOrCreateInputBuffer(indices)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputResult.cached {
		transientBufs = append(transientBufs, inputResult.buffer)
	} else if inputResult.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputResult.gpuData)
	}
	if !indicesResult.cached {
		transientBufs = append(transientBufs, indicesResult.buffer)
	} else if indicesResult.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, indicesResult.gpuData)
	}

	gatherResultSize := uint64(gatherBatchSize) * uint64(outputK) * 4 //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, bufErr := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
		Size:  gatherResultSize,
	})
	if bufErr != nil {
		return nil, fmt.Errorf("runGatherLazy: create result buffer: %w", bufErr)
	}

	// Create uniform buffer. Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	putUint32LE(params[0:4], uint32(gatherBatchSize))
	putUint32LE(params[4:8], uint32(inputDim)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	putUint32LE(params[8:12], uint32(outputK)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)

	sizeInput := uint64(input.ByteSize())     //nolint:gosec // G115: integer overflow conversion int -> uint64
	sizeIndices := uint64(indices.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputResult.buffer, sizeInput),
		bufBinding(indicesResult.buffer, sizeIndices),
		bufBinding(bufferResult, gatherResultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	totalOutput := gatherBatchSize * outputK
	workgroups := uint32((totalOutput + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, gatherResultSize, gatherResultShape, tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runWhereLazy executes conditional selection on GPU and returns a LAZY tensor.
// result[i] = condition[i] != 0 ? x[i] : y[i].
// The result stays on GPU until Data() is called.
//
//nolint:gocyclo,cyclop,funlen,gocognit // Conditional selection with broadcasting has inherent complexity
func (b *Backend) runWhereLazy(condition, x, y *tensor.RawTensor) (*tensor.RawTensor, error) {
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
		condFloat32, err = int32ToFloat32(condition)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errUnsupportedDType(condition.DType())
	}

	// x and y must have same dtype
	if x.DType() != y.DType() {
		return nil, errDTypeMismatch(x.DType(), y.DType())
	}

	// Only float32 and int32 supported
	dtype := x.DType()
	if dtype != tensor.Float32 && dtype != tensor.Int32 {
		return nil, errUnsupportedDType(dtype)
	}

	// Handle broadcasting - compute output shape from all 3 tensors
	outShape := condFloat32.Shape()

	// Broadcast condition with x
	if !condFloat32.Shape().Equal(x.Shape()) {
		broadcastedShape, ok, _ := tensor.BroadcastShapes(condFloat32.Shape(), x.Shape())
		if !ok {
			return nil, errBroadcastFailed(condFloat32.Shape(), x.Shape())
		}
		outShape = broadcastedShape
	}

	// Broadcast outShape with y
	if !outShape.Equal(y.Shape()) {
		broadcastedShape, ok, _ := tensor.BroadcastShapes(outShape, y.Shape())
		if !ok {
			return nil, errBroadcastFailed(outShape, y.Shape())
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

	// Get or create GPU buffers for inputs. Cached CPU tensors reuse the same buffer.
	inputCond := b.getOrCreateInputBuffer(condFloat32)
	inputX := b.getOrCreateInputBuffer(x)
	inputY := b.getOrCreateInputBuffer(y)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputCond.cached {
		transientBufs = append(transientBufs, inputCond.buffer)
	} else if inputCond.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputCond.gpuData)
	}
	if !inputX.cached {
		transientBufs = append(transientBufs, inputX.buffer)
	} else if inputX.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputX.gpuData)
	}
	if !inputY.cached {
		transientBufs = append(transientBufs, inputY.buffer)
	} else if inputY.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputY.gpuData)
	}

	resultSize := uint64(x.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, bufErr := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
		Size:  resultSize,
	})
	if bufErr != nil {
		return nil, fmt.Errorf("runWhereLazy: create result buffer: %w", bufErr)
	}

	// Create uniform buffer. Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)

	condSize := uint64(condFloat32.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputCond.buffer, condSize),
		bufBinding(inputX.buffer, resultSize),
		bufBinding(inputY.buffer, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, outShape, dtype,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runSumLazy executes sum reduction and returns a LAZY tensor.
// For Sum, the result is scalar (4 bytes), so lazy mode has minimal benefit.
// However, this avoids blocking the GPU pipeline during chained operations.
func (b *Backend) runSumLazy(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	dtype := input.DType()
	if dtype != tensor.Float32 && dtype != tensor.Int32 {
		return nil, errUnsupportedDType(dtype)
	}

	numElements := input.NumElements()

	// For small tensors, use CPU (no benefit from lazy mode)
	if numElements < 1024 {
		return b.runSumCPU(input)
	}

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
		return nil, errUnsupportedDType(dtype)
	}

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	// Create input buffer (from lazy tensor if needed)
	bufferInput := b.createBufferFromTensor(input)
	defer bufferInput.Release()

	// Calculate number of workgroups needed
	numWorkgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	partialSumsSize := uint64(numWorkgroups) * 4

	bufferPartialSums, bufErr := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  partialSumsSize,
	})
	if bufErr != nil {
		return nil, fmt.Errorf("runSumLazy: create partial sums buffer: %w", bufErr)
	}
	defer bufferPartialSums.Release()

	// Create uniform buffer for params
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

	// Sum needs immediate readback to aggregate partial results on CPU.
	// Use unified encoder: compute + copy to staging in one submission.
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
		return nil, errUnsupportedDType(dtype)
	}
}

// runClampLazy executes element-wise clamping with lazy result.
// clamp(x, min, max) - data stays on GPU until Data() is called.
func (b *Backend) runClampLazy(input *tensor.RawTensor, minBound, maxBound any) (*tensor.RawTensor, error) {
	dtype := input.DType()
	if dtype != tensor.Float32 && dtype != tensor.Int32 {
		return nil, errUnsupportedDType(dtype)
	}

	numElements := input.NumElements()

	shaderName, shaderCode := selectBinaryShader(dtype, "clamp", clampShader, clampShaderInt32)

	shader := b.compileShader(shaderName, shaderCode)
	pipeline := b.getOrCreatePipeline(shaderName, shader, bglUnary)

	// Get or create GPU buffer for input. Cached CPU tensors reuse the same buffer.
	inputResult := b.getOrCreateInputBuffer(input)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputResult.cached {
		transientBufs = append(transientBufs, inputResult.buffer)
	} else if inputResult.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputResult.gpuData)
	}

	resultSize := uint64(input.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	// CopyDst is retained here for potential future in-place operations.
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("runClampLazy: create result buffer: %w", err)
	}

	// Create params buffer. Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	putUint32LE(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32

	if dtype == tensor.Float32 {
		putFloat32LE(params[4:8], minBound.(float32))
		putFloat32LE(params[8:12], maxBound.(float32))
	} else {
		putInt32LE(params[4:8], minBound.(int32))
		putInt32LE(params[8:12], maxBound.(int32))
	}
	bufferParams := b.createUniformBuffer(params)

	bg := b.createBindGroupFromBuffers(pipeline.layout, []bindGroupBuffer{
		bufBinding(inputResult.buffer, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.addComputePassToEncoder(pipeline.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, input.Shape(), dtype,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// putInt32LE writes an int32 to a byte slice in little-endian order.
func putInt32LE(b []byte, v int32) {
	putUint32LE(b, uint32(v)) //nolint:gosec // G115: safe, int32 fits in uint32
}

// runSelectAddLazy executes SelectAdd on GPU using selectAddShader with a lazy result.
//
// SelectAdd is the Embedding backward kernel: accumulate src rows into dest rows at
// the positions given by 1-D integer indices.
//
// Inputs:
//   - dest:    [numRows, innerSize] float32
//   - indices: [numIndices] int32
//   - src:     [numIndices, innerSize] float32
//
// The shader dispatches one invocation per destination row (per-row approach) to
// avoid the need for f32 atomics, which are not available in WebGPU core WGSL.
func (b *Backend) runSelectAddLazy(dest, indices, src *tensor.RawTensor) (*tensor.RawTensor, error) {
	if dest.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "selectAdd: dest must be float32"}
	}
	if indices.DType() != tensor.Int32 {
		return nil, &lazyError{msg: "selectAdd: indices must be int32"}
	}
	if src.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "selectAdd: src must be float32"}
	}

	destShape := dest.Shape()
	srcShape := src.Shape()
	numRows := uint32(destShape[0])   //nolint:gosec // G115: safe, tensor dims are small positive ints
	numIndices := uint32(srcShape[0]) //nolint:gosec // G115: safe, tensor dims are small positive ints
	innerSize := uint32(destShape[1]) //nolint:gosec // G115: safe, tensor dims are small positive ints

	shader := b.compileShader("selectAdd", selectAddShader)
	entry := b.getOrCreatePipeline("selectAdd", shader, bglScatter)

	// Get or create GPU buffers for inputs. Cached CPU tensors reuse the same buffer.
	inputDest := b.getOrCreateInputBuffer(dest)
	inputIndices := b.getOrCreateInputBuffer(indices)
	inputSrc := b.getOrCreateInputBuffer(src)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputDest.cached {
		transientBufs = append(transientBufs, inputDest.buffer)
	} else if inputDest.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputDest.gpuData)
	}
	if !inputIndices.cached {
		transientBufs = append(transientBufs, inputIndices.buffer)
	} else if inputIndices.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputIndices.gpuData)
	}
	if !inputSrc.cached {
		transientBufs = append(transientBufs, inputSrc.buffer)
	} else if inputSrc.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputSrc.gpuData)
	}

	resultSize := uint64(dest.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runSelectAddLazy: create result buffer: %w", err)
	}

	// Uniform params: num_rows, num_indices, inner_size, _pad (16 bytes).
	// Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	putUint32LE(params[0:4], numRows)
	putUint32LE(params[4:8], numIndices)
	putUint32LE(params[8:12], innerSize)
	bufferParams := b.createUniformBuffer(params)

	sizeDest := uint64(dest.ByteSize())       //nolint:gosec // G115: integer overflow conversion int -> uint64
	sizeIndices := uint64(indices.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	sizeSrc := uint64(src.ByteSize())         //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputDest.buffer, sizeDest),
		bufBinding(inputIndices.buffer, sizeIndices),
		bufBinding(inputSrc.buffer, sizeSrc),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	// One invocation per destination row; 256 threads per workgroup.
	workgroups := (numRows + workgroupSize - 1) / workgroupSize
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, dest.Shape(), tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runScatterAddLazy executes ScatterAdd on GPU using scatterAddShader with a lazy result.
//
// ScatterAdd is the Gather backward kernel: for each element in src, accumulate into
// dest at the position given by the N-D integer indices tensor along the scatter dimension.
//
// Inputs:
//   - dest:    any shape float32
//   - dim:     scatter dimension (normalized, non-negative)
//   - indices: same shape as src, int32
//   - src:     same shape as indices, float32
//
// The shader dispatches one invocation per destination element (per-element approach),
// iterating over all src elements to find matches. No f32 atomics required.
//
//nolint:gocognit,gocyclo,cyclop // scatter-add requires multi-dimensional index arithmetic; complexity is inherent
func (b *Backend) runScatterAddLazy(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) (*tensor.RawTensor, error) {
	if dest.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "scatterAdd: dest must be float32"}
	}
	if indices.DType() != tensor.Int32 {
		return nil, &lazyError{msg: "scatterAdd: indices must be int32"}
	}
	if src.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "scatterAdd: src must be float32"}
	}

	destShape := dest.Shape()
	srcShape := src.Shape()
	ndim := len(destShape)

	if ndim > 6 {
		return nil, &lazyError{msg: "scatterAdd: supports up to 6D tensors"}
	}

	numDestElements := uint32(dest.NumElements()) //nolint:gosec // G115: safe, element counts are bounded
	numSrcElements := uint32(src.NumElements())   //nolint:gosec // G115: safe, element counts are bounded

	shader := b.compileShader("scatterAdd", scatterAddShader)
	entry := b.getOrCreatePipeline("scatterAdd", shader, bglScatter)

	// Get or create GPU buffers for inputs. Cached CPU tensors reuse the same buffer.
	inputDest := b.getOrCreateInputBuffer(dest)
	inputIndices := b.getOrCreateInputBuffer(indices)
	inputSrc := b.getOrCreateInputBuffer(src)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputDest.cached {
		transientBufs = append(transientBufs, inputDest.buffer)
	} else if inputDest.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputDest.gpuData)
	}
	if !inputIndices.cached {
		transientBufs = append(transientBufs, inputIndices.buffer)
	} else if inputIndices.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputIndices.gpuData)
	}
	if !inputSrc.cached {
		transientBufs = append(transientBufs, inputSrc.buffer)
	} else if inputSrc.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputSrc.gpuData)
	}

	resultSize := uint64(dest.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runScatterAddLazy: create result buffer: %w", err)
	}

	// Build uniform params (24 u32 = 96 bytes, padded to 96 for alignment).
	// Layout: num_dest_elements, num_src_elements, scatter_dim, ndim,
	//         dest_shape[6], dest_strides[6], src_strides[6], _pad[2].
	// Ownership transfers to addComputePassToEncoder.
	destStrides := destShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()

	const paramsU32Count = 24 // 96 bytes total
	params := make([]byte, paramsU32Count*4)
	putUint32LE(params[0:4], numDestElements)
	putUint32LE(params[4:8], numSrcElements)
	putUint32LE(params[8:12], uint32(dim)) //nolint:gosec // G115: dim is non-negative and small
	putUint32LE(params[12:16], uint32(ndim))

	// dest_shape[0..5] — pad with 1 for unused dimensions.
	for i := 0; i < 6; i++ {
		if i < ndim {
			putUint32LE(params[16+i*4:20+i*4], uint32(destShape[i])) //nolint:gosec // G115: shape dim is small positive
		} else {
			putUint32LE(params[16+i*4:20+i*4], 1)
		}
	}

	// dest_strides[0..5] — pad with 1.
	for i := 0; i < 6; i++ {
		if i < ndim {
			putUint32LE(params[40+i*4:44+i*4], uint32(destStrides[i])) //nolint:gosec // G115: stride is small positive
		} else {
			putUint32LE(params[40+i*4:44+i*4], 1)
		}
	}

	// src_strides[0..5] — pad with 1.
	for i := 0; i < 6; i++ {
		if i < ndim {
			putUint32LE(params[64+i*4:68+i*4], uint32(srcStrides[i])) //nolint:gosec // G115: stride is small positive
		} else {
			putUint32LE(params[64+i*4:68+i*4], 1)
		}
	}
	// _pad[0], _pad[1] at offsets 88 and 92 remain zero.

	paramsSize := uint64(len(params))
	bufferParams := b.createUniformBuffer(params)

	sizeDest := uint64(dest.ByteSize())       //nolint:gosec // G115: integer overflow conversion int -> uint64
	sizeIndices := uint64(indices.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	sizeSrc := uint64(src.ByteSize())         //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputDest.buffer, sizeDest),
		bufBinding(inputIndices.buffer, sizeIndices),
		bufBinding(inputSrc.buffer, sizeSrc),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, paramsSize),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	// One invocation per destination element.
	workgroups := (numDestElements + workgroupSize - 1) / workgroupSize
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, dest.Shape(), tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// =============================================================================
// Lazy paths for shape/reduction ops (TASK-148)
// =============================================================================

// runReshapeLazy returns a lazy tensor with the given shape backed by a GPU-to-GPU
// copy of the source buffer. This is zero-CPU: no data ever touches the host.
//
// A true zero-copy view (same buffer, different shape metadata) would require
// reference-counted buffer sharing between two LazyGPUData objects. The current
// LazyGPUData owns exactly one buffer and releases it on finalize, so sharing is
// not possible without a refcount layer. Instead we use CopyBufferToBuffer (which
// stays on the GPU command queue) — the copy is cheap and avoids the 335 MB/step
// CPU allocation that the old path triggered.
func (b *Backend) runReshapeLazy(t *tensor.RawTensor, newShape tensor.Shape) (*tensor.RawTensor, error) {
	if t.DType() != tensor.Float32 && t.DType() != tensor.Int32 {
		return nil, &lazyError{msg: "reshape: only float32 and int32 are supported"}
	}

	// GPU-to-GPU copy: flushes pending commands, then CopyBufferToBuffer.
	// Ownership of dstBuffer transfers to the new LazyGPUData below.
	gpuData := t.GPUData()
	if gpuData == nil || gpuData.IsRealized() {
		return nil, &lazyError{msg: "reshape: source tensor has no unrealized GPU data"}
	}

	bp := gpuData.BufferPtr()
	if bp == nil {
		return nil, &lazyError{msg: "reshape: source GPU buffer already released"}
	}
	srcBuffer := (*wgpu.Buffer)(bp)
	size := gpuData.Size()

	dstBuffer := b.copyGPUBuffer(srcBuffer, size)
	runtime.KeepAlive(gpuData)

	return b.createLazyResult(dstBuffer, size, newShape, t.DType())
}

// runSumDimLazy executes sum reduction along any dimension of an N-D tensor
// using the sumDimGeneralShader. Returns a lazy GPU tensor.
//
// The shader decomposes the tensor into (outer_size, dim_size, inner_size) and
// dispatches one thread per output element, each summing dim_size values.
//
// keepDim=true pads the reduced dimension with size 1 (shape unchanged in rank).
// keepDim=false removes the reduced dimension from the output shape.
func (b *Backend) runSumDimLazy(x *tensor.RawTensor, dim int, keepDim bool) (*tensor.RawTensor, error) {
	if x.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "sumDim: only float32 is supported"}
	}

	shape := x.Shape()
	ndim := len(shape)

	// Compute outer_size, dim_size, inner_size.
	outerSize := 1
	for i := 0; i < dim; i++ {
		outerSize *= shape[i]
	}
	dimSize := shape[dim]
	innerSize := 1
	for i := dim + 1; i < ndim; i++ {
		innerSize *= shape[i]
	}

	// Build output shape.
	var outShape tensor.Shape
	if keepDim {
		outShape = shape.Clone()
		outShape[dim] = 1
	} else {
		outShape = make(tensor.Shape, 0, ndim-1)
		for i := 0; i < ndim; i++ {
			if i != dim {
				outShape = append(outShape, shape[i])
			}
		}
		if len(outShape) == 0 {
			outShape = tensor.Shape{} // scalar
		}
	}

	numOutputElements := outerSize * innerSize
	resultSize := uint64(numOutputElements * 4) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// sumDimLazyShader params: num_output, dim_size, inner_size, _pad (16 bytes).
	// num_output = outer_size * inner_size = total output elements.
	shader := b.compileShader("sumDimLazy", sumDimLazyShader)
	entry := b.getOrCreatePipeline("sumDimLazy", shader, bglUnary)

	// Get or create GPU buffer for input. Cached CPU tensors reuse the same buffer.
	inputX := b.getOrCreateInputBuffer(x)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputX.cached {
		transientBufs = append(transientBufs, inputX.buffer)
	} else if inputX.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputX.gpuData)
	}

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runSumDimLazy: create result buffer: %w", err)
	}

	// Params: num_output, dim_size, inner_size, _pad (16 bytes, std140).
	// Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	putUint32LE(params[0:4], uint32(numOutputElements)) //nolint:gosec // G115: safe, numOutputElements = outerSize*innerSize bounded by tensor dims
	putUint32LE(params[4:8], uint32(dimSize))           //nolint:gosec // G115: safe, tensor dim size bounded by int max
	putUint32LE(params[8:12], uint32(innerSize))
	bufferParams := b.createUniformBuffer(params)

	inputSize := uint64(x.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputX.buffer, inputSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	workgroups := uint32((numOutputElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, outShape, tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}

// runCatLazy concatenates a list of tensors along dim, returning a lazy GPU tensor.
//
// Each input tensor is dispatched independently via catShader, which maps each
// element from the input into the correct slot of the pre-allocated output buffer.
// All dispatches share the same output buffer (owned by the result LazyGPUData).
//
// Limitation: all inputs must be float32.
//
//nolint:gocognit,gocyclo,cyclop,funlen // per-tensor dispatch loop with lock-managed encoder batch has inherent branching complexity
func (b *Backend) runCatLazy(tensors []*tensor.RawTensor, dim int) (*tensor.RawTensor, error) {
	if len(tensors) == 0 {
		return nil, &lazyError{msg: "cat: at least one tensor required"}
	}
	// Validate: all float32, same ndim, compatible shapes.
	dtype := tensors[0].DType()
	if dtype != tensor.Float32 {
		return nil, &lazyError{msg: "cat: only float32 is supported"}
	}
	shape0 := tensors[0].Shape()
	ndim := len(shape0)

	totalDim := 0
	for _, t := range tensors {
		if t.DType() != dtype {
			return nil, &lazyError{msg: "cat: all tensors must have same dtype"}
		}
		totalDim += t.Shape()[dim]
	}

	// Build output shape.
	outShape := shape0.Clone()
	outShape[dim] = totalDim

	// inner_size = product of dimensions after dim (same stride for all inputs).
	innerSize := 1
	for i := dim + 1; i < ndim; i++ {
		innerSize *= shape0[i]
	}

	resultNumElements := outShape.NumElements()
	resultSize := uint64(resultNumElements * 4) //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Allocate the single shared output buffer. Ownership transfers to LazyGPUData below.
	// CopyDst is needed because we write into it from multiple compute passes.
	// Cannot use gpuPool here: the buffer must survive across all per-input dispatches.
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		return nil, fmt.Errorf("runCatLazy: create result buffer: %w", err)
	}

	shader := b.compileShader("cat", catShader)
	entry := b.getOrCreatePipeline("cat", shader, bglUnary)

	// Track all lazy input gpuDatas to keep them alive until Submit.
	var allLazyDatas []*tensor.LazyGPUData

	outDimSize := uint32(totalDim)

	dimOffset := uint32(0)
	for _, t := range tensors {
		tShape := t.Shape()
		dimSizeIn := uint32(tShape[dim]) //nolint:gosec // G115: safe, tensor dims are positive ints bounded by allocated slice length
		innerSizeIn := uint32(innerSize)

		// outer_stride_in = dim_size_in * inner_size_in (elements per "outer slice" in this input).
		outerStrideIn := dimSizeIn * innerSizeIn
		numElementsIn := uint32(t.NumElements()) //nolint:gosec // G115: safe, element counts are bounded

		inputT := b.getOrCreateInputBuffer(t)
		if inputT.cached && inputT.gpuData != nil {
			allLazyDatas = append(allLazyDatas, inputT.gpuData)
		}
		// Non-cached transient buffers are registered in iterTransient below.

		// Cat shader params (32 bytes, 8 x u32):
		//   num_elements, out_dim_size, dim_offset, dim_stride_out, inner_size_in, dim_size_in, outer_stride_in, _pad
		params := make([]byte, 32)
		putUint32LE(params[0:4], numElementsIn)
		putUint32LE(params[4:8], outDimSize)
		putUint32LE(params[8:12], dimOffset)
		putUint32LE(params[12:16], innerSizeIn)
		putUint32LE(params[16:20], innerSizeIn)
		putUint32LE(params[20:24], dimSizeIn)
		putUint32LE(params[24:28], outerStrideIn)
		// _pad at [28:32] is zero already.
		bufferParams := b.createUniformBuffer(params)

		inputSize := uint64(t.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
		bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
			bufBinding(inputT.buffer, inputSize),
			bufBinding(bufferResult, resultSize),
			bufBinding(bufferParams, 32),
		})
		// NO defer bg.Release() — ownership transfers to encoder batch below.

		workgroups := (numElementsIn + workgroupSize - 1) / uint32(workgroupSize)

		// Encode the compute pass directly into the shared encoder batch.
		// We cannot use addComputePassToEncoder here because that would call
		// createLazyResult which takes ownership of bufferResult — but bufferResult
		// must be shared across all per-input dispatches and only wrapped in a
		// LazyGPUData once at the end.
		//
		// Instead: encode the pass manually and register params+bg+lazyDatas
		// in the batch immediately so they survive any mid-loop auto-flush.
		b.pendingMu.Lock()
		enc := b.getOrCreateEncoderLocked()
		computePass, cpErr := enc.BeginComputePass(nil)
		if cpErr != nil {
			b.pendingMu.Unlock()
			bg.Release()
			bufferParams.Release()
			if !inputT.cached {
				inputT.buffer.Release()
			}
			bufferResult.Release()
			return nil, fmt.Errorf("runCatLazy: BeginComputePass: %w", cpErr)
		}
		computePass.SetPipeline(entry.pipeline)
		computePass.SetBindGroup(0, bg, nil)
		computePass.Dispatch(workgroups, 1, 1)
		if endErr := computePass.End(); endErr != nil {
			b.pendingMu.Unlock()
			bg.Release()
			bufferResult.Release()
			panic(fmt.Sprintf("webgpu: runCatLazy: compute pass End: %v", endErr))
		}

		// Register resources in the active batch immediately so they are tracked
		// through any auto-flush that fires during this loop iteration.
		// resultBufs holds params + transient inputs; bindGroups holds bg.
		// lazyDatas keeps GPU input tensors alive until Submit.
		var iterTransient []*wgpu.Buffer
		iterTransient = append(iterTransient, bufferParams)
		if !inputT.cached {
			iterTransient = append(iterTransient, inputT.buffer)
		}
		b.activeBatch.resultBufs = append(b.activeBatch.resultBufs, iterTransient...)
		b.activeBatch.bindGroups = append(b.activeBatch.bindGroups, bg)
		b.activeBatch.lazyDatas = append(b.activeBatch.lazyDatas, allLazyDatas...)
		allLazyDatas = nil // transferred above; reset to avoid double-append on next iteration
		b.activeBatch.count++
		b.activeBatch.allocBytes += resultSize / uint64(len(tensors)) // approximate

		shouldFlush := b.activeBatch.count >= maxPendingBeforeFlush || b.activeBatch.allocBytes >= maxBatchAllocBytes
		if shouldFlush {
			b.finishActiveBatchLocked()
		}
		b.pendingMu.Unlock()
		if shouldFlush {
			b.flushCommands()
		}

		dimOffset += dimSizeIn
	}

	// All per-input params buffers are now registered in their respective batches.
	// Wrap the shared output buffer in a lazy tensor. Ownership of bufferResult
	// transfers to the LazyGPUData created inside createLazyResult.
	return b.createLazyResult(bufferResult, resultSize, outShape, dtype)
}

// runChunkLazy splits a tensor into n equal parts along dim, returning lazy GPU tensors.
//
// Each output chunk is dispatched via chunkShader, copying the appropriate slice
// of the input into a freshly-allocated output buffer.
//
// Limitation: only float32 is supported.
//
//nolint:gocognit // per-chunk dispatch loop with error recovery inherently requires multiple branches
func (b *Backend) runChunkLazy(x *tensor.RawTensor, n, dim int) ([]*tensor.RawTensor, error) {
	if x.DType() != tensor.Float32 {
		return nil, &lazyError{msg: "chunk: only float32 is supported"}
	}

	shape := x.Shape()
	ndim := len(shape)
	dimSize := shape[dim]
	chunkSize := dimSize / n

	chunkShape := shape.Clone()
	chunkShape[dim] = chunkSize

	// inner_size = product of dimensions after dim.
	innerSize := 1
	for i := dim + 1; i < ndim; i++ {
		innerSize *= shape[i]
	}
	inDimSize := uint32(dimSize) //nolint:gosec // G115: safe, tensor dim bounded by int max
	innerSizeU := uint32(innerSize)

	numChunkElements := chunkShape.NumElements()
	chunkResultSize := uint64(numChunkElements * 4) //nolint:gosec // G115: integer overflow conversion int -> uint64
	numElementsU := uint32(numChunkElements)        //nolint:gosec // G115: safe, element counts are bounded

	shader := b.compileShader("chunk", chunkShader)
	entry := b.getOrCreatePipeline("chunk", shader, bglUnary)

	// Get or create GPU buffer for input. Shared (read-only) across all chunk dispatches.
	// For cached buffers the Backend owns the lifetime; for transient GPU buffers
	// we let each per-chunk lazyResources entry hold a reference so the buffer
	// stays alive until after Submit.
	inputX := b.getOrCreateInputBuffer(x)
	inputSize := uint64(x.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64

	results := make([]*tensor.RawTensor, n)

	// releaseResults releases any already-created chunk results on error.
	releaseResults := func(upTo int) {
		for _, r := range results[:upTo] {
			if r != nil {
				if gd := r.GPUData(); gd != nil {
					gd.Release()
				}
			}
		}
	}

	for i := 0; i < n; i++ {
		chunkOffset := uint32(i * chunkSize)             //nolint:gosec // G115: safe, offset bounded by tensor dim
		outerStrideOut := uint32(chunkSize) * innerSizeU //nolint:gosec // G115: safe product of small positive ints

		// Result buffer for this chunk; ownership transfers to LazyGPUData.
		bufferResult, err := b.gpuPool.Acquire(chunkResultSize)
		if err != nil {
			releaseResults(i)
			if !inputX.cached {
				inputX.buffer.Release()
			}
			return nil, fmt.Errorf("runChunkLazy: create result buffer for chunk %d: %w", i, err)
		}

		// Chunk shader params (32 bytes, 8 x u32):
		//   num_elements, in_dim_size, chunk_offset, dim_stride_in, inner_size, chunk_size, outer_stride_out, _pad
		params := make([]byte, 32)
		putUint32LE(params[0:4], numElementsU)
		putUint32LE(params[4:8], inDimSize)
		putUint32LE(params[8:12], chunkOffset)
		putUint32LE(params[12:16], innerSizeU)
		putUint32LE(params[16:20], innerSizeU)
		putUint32LE(params[20:24], uint32(chunkSize)) //nolint:gosec // G115: safe, chunkSize is small positive int
		putUint32LE(params[24:28], outerStrideOut)
		// _pad at [28:32] is zero already.
		bufferParams := b.createUniformBuffer(params)

		bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
			bufBinding(inputX.buffer, inputSize),
			bufBinding(bufferResult, chunkResultSize),
			bufBinding(bufferParams, 32),
		})
		// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

		// For non-cached (transient GPU) input buffers, each chunk dispatch gets its
		// own reference in lazyResources.buffers so the buffer stays alive until Submit.
		// For cached buffers the Backend holds the lifetime; track gpuData instead.
		var transientBufs []*wgpu.Buffer
		var lazyDatas []*tensor.LazyGPUData
		if !inputX.cached {
			transientBufs = append(transientBufs, inputX.buffer)
		} else if inputX.gpuData != nil {
			lazyDatas = append(lazyDatas, inputX.gpuData)
		}

		workgroups := (numElementsU + uint32(workgroupSize) - 1) / uint32(workgroupSize)
		result, encErr := b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1,
			bufferResult, chunkResultSize, chunkShape.Clone(), tensor.Float32,
			lazyResources{
				buffers:    append(transientBufs, bufferParams),
				bindGroups: []*wgpu.BindGroup{},
				lazyDatas:  lazyDatas,
			})
		if encErr != nil {
			bufferResult.Release()
			releaseResults(i)
			return nil, fmt.Errorf("runChunkLazy: encode chunk %d: %w", i, encErr)
		}
		results[i] = result
	}

	return results, nil
}

// runEmbeddingLazy performs embedding lookup on GPU and returns a LAZY tensor.
// weight: [num_embeddings, embedding_dim] float32, indices: [...] int32.
// Returns: [...indices_shape, embedding_dim] float32.
//
// Uses the same shader and bind group layout as runEmbedding but routes through
// the shared encoder accumulator (addComputePassToEncoder) so the result stays
// on GPU until Data() is called, eliminating the GPU→CPU readback per step.
func (b *Backend) runEmbeddingLazy(weight, indices *tensor.RawTensor) (*tensor.RawTensor, error) {
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

	// Get or create GPU buffers for inputs. Weight matrices are persistent (cached:true
	// after first call). Indices are per-token so typically CPU-uploaded each forward pass.
	inputWeight := b.getOrCreateInputBuffer(weight)
	inputIndices := b.getOrCreateInputBuffer(indices)

	// Collect transient input buffers and lazy input gpuDatas for Submit-safety.
	var transientBufs []*wgpu.Buffer
	var inputLazyDatas []*tensor.LazyGPUData
	if !inputWeight.cached {
		transientBufs = append(transientBufs, inputWeight.buffer)
	} else if inputWeight.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputWeight.gpuData)
	}
	if !inputIndices.cached {
		transientBufs = append(transientBufs, inputIndices.buffer)
	} else if inputIndices.gpuData != nil {
		inputLazyDatas = append(inputLazyDatas, inputIndices.gpuData)
	}

	resultSize := uint64(numIndices) * uint64(embeddingDim) * 4 //nolint:gosec // G115: integer overflow conversion int -> uint64

	// Result buffer: written by the compute shader; ownership transfers to LazyGPUData.
	bufferResult, err := b.gpuPool.Acquire(resultSize)
	if err != nil {
		return nil, fmt.Errorf("runEmbeddingLazy: create result buffer: %w", err)
	}

	// Params layout matches runEmbedding: num_indices, embedding_dim, num_embeddings, _pad (16 bytes).
	// Ownership transfers to addComputePassToEncoder.
	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numIndices))     //nolint:gosec // G115: integer overflow conversion int -> uint32
	binary.LittleEndian.PutUint32(params[4:8], uint32(embeddingDim))   //nolint:gosec // G115: safe, embedding dimensions are non-negative and fit in uint32
	binary.LittleEndian.PutUint32(params[8:12], uint32(numEmbeddings)) //nolint:gosec // G115: safe, embedding count is non-negative and fits in uint32
	bufferParams := b.createUniformBuffer(params)

	weightSize := uint64(weight.ByteSize())   //nolint:gosec // G115: integer overflow conversion int -> uint64
	indicesSize := uint64(indices.ByteSize()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(inputWeight.buffer, weightSize),
		bufBinding(inputIndices.buffer, indicesSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	// NO defer bg.Release() — ownership transfers to encoder batch via lazyResources.

	totalElements := numIndices * embeddingDim
	workgroups := uint32((totalElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	return b.addComputePassToEncoder(entry.pipeline, bg, workgroups, 1, 1, bufferResult, resultSize, outputShape, tensor.Float32,
		lazyResources{
			buffers:    append(transientBufs, bufferParams),
			bindGroups: []*wgpu.BindGroup{},
			lazyDatas:  inputLazyDatas,
		})
}
