//go:build windows || linux

// Package webgpu implements the WebGPU backend for GPU-accelerated tensor operations.
package webgpu

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
	"github.com/gogpu/gputypes"
	wgpu "github.com/gogpu/wgpu"
)

// AddGPU performs element-wise addition on GPU tensors.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) AddGPU(a, c *GPUTensor) *GPUTensor {
	return b.runBinaryOpGPU(a, c, "add", addShader)
}

// SubGPU performs element-wise subtraction on GPU tensors.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) SubGPU(a, c *GPUTensor) *GPUTensor {
	return b.runBinaryOpGPU(a, c, "sub", subShader)
}

// MulGPU performs element-wise multiplication on GPU tensors.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) MulGPU(a, c *GPUTensor) *GPUTensor {
	return b.runBinaryOpGPU(a, c, "mul", mulShader)
}

// DivGPU performs element-wise division on GPU tensors.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) DivGPU(a, c *GPUTensor) *GPUTensor {
	return b.runBinaryOpGPU(a, c, "div", divShader)
}

// runBinaryOpGPU executes binary operations on GPU tensors without CPU transfer.
// This is the core primitive for lazy GPU operations.
func (b *Backend) runBinaryOpGPU(a, c *GPUTensor, opName, shaderCode string) *GPUTensor {
	// Validate shapes
	if !a.shape.Equal(c.shape) {
		panic(fmt.Sprintf("webgpu: %s: shape mismatch: %v vs %v", opName, a.shape, c.shape))
	}

	// Validate dtypes
	if a.dtype != c.dtype {
		panic(fmt.Sprintf("webgpu: %s: dtype mismatch: %s vs %s", opName, a.dtype, c.dtype))
	}

	// Only float32 and int32 supported for now
	if a.dtype != tensor.Float32 && a.dtype != tensor.Int32 {
		panic(fmt.Sprintf("webgpu: %s: only float32 and int32 supported, got %s", opName, a.dtype))
	}

	// Select shader based on dtype
	shaderName := opName
	if a.dtype == tensor.Int32 {
		shaderName = opName + "Int32"
		switch opName {
		case "add":
			shaderCode = addShaderInt32
		case "sub":
			shaderCode = subShaderInt32
		case "mul":
			shaderCode = mulShaderInt32
		case "div":
			shaderCode = divShaderInt32
		default:
			panic(fmt.Sprintf("webgpu: %s: no int32 shader available", opName))
		}
	}

	numElements := a.NumElements()

	shader := b.compileShader(shaderName, shaderCode)
	entry := b.getOrCreatePipeline(shaderName, shader, bglBinary)

	// Create output buffer (stays on GPU — caller owns it via GPUTensor).
	resultSize := a.ByteSize()
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: runBinaryOpGPU: failed to create result buffer: %v", err))
	}

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(a.buffer, resultSize),
		bufBinding(c.buffer, resultSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	b.execComputePass(entry.pipeline, bg, workgroups, 1, 1)

	return &GPUTensor{
		buffer:     bufferResult,
		shape:      a.shape,
		dtype:      a.dtype,
		strides:    a.strides,
		backend:    b,
		computed:   true,
		bufferSize: resultSize,
	}
}

// MatMulGPU performs matrix multiplication on GPU tensors.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) MatMulGPU(a, c *GPUTensor) *GPUTensor {
	// Validate shapes (must be 2D)
	if len(a.shape) != 2 || len(c.shape) != 2 {
		panic(fmt.Sprintf("webgpu: MatMulGPU: expected 2D tensors, got shapes %v and %v", a.shape, c.shape))
	}

	// Validate matrix dimensions
	if a.shape[1] != c.shape[0] {
		panic(fmt.Sprintf("webgpu: MatMulGPU: incompatible matrix dimensions: [%d, %d] @ [%d, %d]",
			a.shape[0], a.shape[1], c.shape[0], c.shape[1]))
	}

	m := a.shape[0]
	k := a.shape[1]
	n := c.shape[1]

	// Output shape: [m, n]
	outShape := tensor.Shape{m, n}

	shader := b.compileShader("matmul", matmulShader)
	entry := b.getOrCreatePipeline("matmul", shader, bglBinary)

	resultSize := uint64(m * n * a.dtype.Size()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: MatMulGPU: failed to create result buffer: %v", err))
	}

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(m))  //nolint:gosec // G115: safe, tensor dims are small positive ints
	binary.LittleEndian.PutUint32(params[4:8], uint32(k))  //nolint:gosec // G115: safe, tensor dims are small positive ints
	binary.LittleEndian.PutUint32(params[8:12], uint32(n)) //nolint:gosec // G115: safe, tensor dims are small positive ints
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(a.buffer, a.bufferSize),
		bufBinding(c.buffer, c.bufferSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	const tileSize = 16
	workgroupsX := uint32((m + tileSize - 1) / tileSize) //nolint:gosec // G115: safe, tensor dims are small positive ints
	workgroupsY := uint32((n + tileSize - 1) / tileSize) //nolint:gosec // G115: safe, tensor dims are small positive ints
	b.execComputePass(entry.pipeline, bg, workgroupsX, workgroupsY, 1)

	return &GPUTensor{
		buffer:     bufferResult,
		shape:      outShape,
		dtype:      a.dtype,
		strides:    outShape.ComputeStrides(),
		backend:    b,
		computed:   true,
		bufferSize: resultSize,
	}
}

// TransposeGPU transposes a 2D tensor on GPU.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) TransposeGPU(t *GPUTensor, axes ...int) *GPUTensor {
	shape := t.shape
	ndim := len(shape)

	// Validate 2D for now
	if ndim != 2 {
		panic(fmt.Sprintf("webgpu: TransposeGPU: only 2D tensors supported for now, got %dD", ndim))
	}

	// Validate axes
	if len(axes) > 0 && (len(axes) != 2 || !isValid2DAxes(axes)) {
		panic("webgpu: TransposeGPU: invalid axes for 2D tensor")
	}

	m := shape[0]
	n := shape[1]

	// Output shape: [n, m]
	outShape := tensor.Shape{n, m}

	shader := b.compileShader("transpose", transposeShader)
	entry := b.getOrCreatePipeline("transpose", shader, bglUnary)

	resultSize := uint64(m * n * t.dtype.Size()) //nolint:gosec // G115: integer overflow conversion int -> uint64
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: TransposeGPU: failed to create result buffer: %v", err))
	}

	params := make([]byte, 16)
	binary.LittleEndian.PutUint32(params[0:4], uint32(m)) //nolint:gosec // G115: safe, tensor dims are small positive ints
	binary.LittleEndian.PutUint32(params[4:8], uint32(n)) //nolint:gosec // G115: safe, tensor dims are small positive ints
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(t.buffer, t.bufferSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	const tileSize = 16
	workgroupsX := uint32((n + tileSize - 1) / tileSize) //nolint:gosec // G115: safe, tensor dims are small positive ints
	workgroupsY := uint32((m + tileSize - 1) / tileSize) //nolint:gosec // G115: safe, tensor dims are small positive ints
	b.execComputePass(entry.pipeline, bg, workgroupsX, workgroupsY, 1)

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

// ReLUGPU applies ReLU activation on GPU: max(0, x).
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) ReLUGPU(t *GPUTensor) *GPUTensor {
	return b.runUnaryOpGPU(t, "relu", reluShader)
}

// SigmoidGPU applies sigmoid activation on GPU: 1 / (1 + exp(-x)).
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) SigmoidGPU(t *GPUTensor) *GPUTensor {
	return b.runUnaryOpGPU(t, "sigmoid", sigmoidShader)
}

// TanhGPU applies tanh activation on GPU.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) TanhGPU(t *GPUTensor) *GPUTensor {
	return b.runUnaryOpGPU(t, "tanh", tanhShader)
}

// ErfGPU applies erf activation on GPU.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) ErfGPU(t *GPUTensor) *GPUTensor {
	return b.runUnaryOpGPU(t, "erf", erfShader)
}

// SignGPU applies sign activation on GPU.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) SignGPU(t *GPUTensor) *GPUTensor {
	return b.runUnaryOpGPU(t, "sign", signShader)
}

// AbsGPU applies absolute value activation on GPU.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) AbsGPU(t *GPUTensor) *GPUTensor {
	return b.runUnaryOpGPU(t, "abs", absShader)
}

// ClampGPU applies clamp activation on GPU: clamp(x, minValue, maxValue).
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) ClampGPU(t *GPUTensor, minValue, maxValue any) *GPUTensor {
	// Validate dtype
	if t.dtype != tensor.Float32 && t.dtype != tensor.Int32 {
		panic(fmt.Sprintf("webgpu: ClampGPU: only float32 and int32 supported, got %s", t.dtype))
	}

	shaderName, shaderCode := selectBinaryShader(t.dtype, "clamp", clampShader, clampShaderInt32)

	numElements := t.NumElements()

	// Compile shader
	shader := b.compileShader(shaderName, shaderCode)

	// Get or create pipeline
	pipeline := b.getOrCreatePipeline(shaderName, shader, bglBinary)

	// Create output buffer (stays on GPU!)
	resultSize := t.ByteSize()
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: ClampGPU: failed to create result buffer: %v", err))
	}

	// Create uniform buffer for params (size: u32, min: f32/i32, max: f32/i32)
	params := make([]byte, 16)                                      // 16-byte aligned
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32

	if t.dtype == tensor.Float32 {
		minVal := tensor.CheckScalarDType[float32](minValue)
		maxVal := tensor.CheckScalarDType[float32](maxValue)
		binary.LittleEndian.PutUint32(params[4:8], math.Float32bits(minVal))
		binary.LittleEndian.PutUint32(params[8:12], math.Float32bits(maxVal))
	} else {
		minVal := tensor.CheckScalarDType[int32](minValue)
		maxVal := tensor.CheckScalarDType[int32](maxValue)
		binary.LittleEndian.PutUint32(params[4:8], uint32(minVal))  //nolint:gosec
		binary.LittleEndian.PutUint32(params[8:12], uint32(maxVal)) //nolint:gosec
	}

	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	// Get bind group layout and create bind group
	bg := b.createBindGroupFromBuffers(pipeline.layout, []bindGroupBuffer{
		bufBinding(t.buffer, t.bufferSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	b.execComputePass(pipeline.pipeline, bg, workgroups, 1, 1)

	// Return GPUTensor (NO readBuffer!)
	return &GPUTensor{
		buffer:     bufferResult,
		bufferSize: resultSize,
		shape:      t.shape,
		dtype:      t.dtype,
		backend:    b,
	}
}

// SoftmaxGPU applies softmax activation along the specified dimension.
// For now, only last dimension (dim=-1) is supported efficiently on GPU.
// Data stays on GPU - no CPU transfer occurs.
func (b *Backend) SoftmaxGPU(t *GPUTensor, dim int) *GPUTensor {
	shape := t.shape
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	if dim < 0 || dim >= ndim {
		panic("webgpu: SoftmaxGPU: dimension out of range")
	}

	// For now, only support last dimension
	if dim != ndim-1 {
		panic("webgpu: SoftmaxGPU: only last dimension supported for now")
	}

	// Calculate batch size and feature size
	batchSize := 1
	for i := 0; i < ndim-1; i++ {
		batchSize *= shape[i]
	}
	featureSize := shape[ndim-1]

	// Compile shader
	shader := b.compileShader("softmax", softmaxShader)
	entry := b.getOrCreatePipeline("softmax", shader, bglUnary)

	// Create output buffer (stays on GPU!)
	resultSize := t.ByteSize()
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: SoftmaxGPU: failed to create result buffer: %v", err))
	}

	// Create uniform buffer for params
	params := make([]byte, 16) // 16-byte aligned
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

	// Dispatch workgroups: one per batch element
	workgroups := uint32(batchSize)
	b.execComputePass(entry.pipeline, bg, workgroups, 1, 1)

	// Return GPUTensor (NO readBuffer!)
	return &GPUTensor{
		buffer:     bufferResult,
		shape:      t.shape,
		dtype:      t.dtype,
		strides:    t.strides,
		backend:    b,
		computed:   true,
		bufferSize: resultSize,
	}
}

// UploadTensor uploads a CPU tensor to GPU memory.
// Returns a GPUTensor that can be used for lazy GPU operations.
func (b *Backend) UploadTensor(raw *tensor.RawTensor) *GPUTensor {
	// Calculate aligned buffer size (WebGPU requires 4-byte alignment)
	numElements := raw.NumElements()
	bytesPerElement := raw.DType().Size()
	actualByteSize := numElements * bytesPerElement

	alignedSize := uint64((actualByteSize + 3) &^ 3) //nolint:gosec // Round up to 4-byte boundary

	// Create GPU buffer using createBuffer which handles MappedAtCreation correctly.
	buffer := b.createBuffer(
		raw.Data()[:actualByteSize],
		gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc|gputypes.BufferUsageCopyDst,
	)

	return &GPUTensor{
		buffer:     buffer,
		shape:      raw.Shape(),
		dtype:      raw.DType(),
		strides:    raw.Shape().ComputeStrides(),
		backend:    b,
		computed:   true,
		bufferSize: alignedSize,
	}
}

// runUnaryOpGPU executes unary operations on GPU tensors without CPU transfer.
func (b *Backend) runUnaryOpGPU(t *GPUTensor, opName, shaderCode string) *GPUTensor {
	numElements := t.NumElements()

	shader := b.compileShader(opName, shaderCode)
	entry := b.getOrCreatePipeline(opName, shader, bglUnary)

	// Create output buffer (stays on GPU!)
	resultSize := t.ByteSize()
	bufferResult, err := b.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
		Size:  resultSize,
	})
	if err != nil {
		panic(fmt.Sprintf("webgpu: runUnaryOpGPU: failed to create result buffer: %v", err))
	}

	// Create uniform buffer for params (size: u32)
	params := make([]byte, 16)                                      // 16-byte aligned
	binary.LittleEndian.PutUint32(params[0:4], uint32(numElements)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	bufferParams := b.createUniformBuffer(params)
	defer bufferParams.Release()

	bg := b.createBindGroupFromBuffers(entry.layout, []bindGroupBuffer{
		bufBinding(t.buffer, t.bufferSize),
		bufBinding(bufferResult, resultSize),
		bufBinding(bufferParams, 16),
	})
	defer bg.Release()

	// Calculate workgroup count: ceil(numElements / workgroupSize)
	workgroups := uint32((numElements + workgroupSize - 1) / workgroupSize) //nolint:gosec // G115: integer overflow conversion int -> uint32
	b.execComputePass(entry.pipeline, bg, workgroups, 1, 1)

	// Return GPUTensor (NO readBuffer!)
	return &GPUTensor{
		buffer:     bufferResult,
		shape:      t.shape,
		dtype:      t.dtype,
		strides:    t.strides,
		backend:    b,
		computed:   true,
		bufferSize: resultSize,
	}
}
