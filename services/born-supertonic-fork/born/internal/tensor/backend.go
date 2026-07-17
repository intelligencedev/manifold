package tensor

// Backend defines the interface that all compute backends must implement.
// Backends handle the actual computation for tensor operations.
//
// Implementations:
//   - CPU: Pure Go with SIMD optimizations (TASK-003)
//   - CUDA: NVIDIA GPU via driver API (Phase 2)
//   - Vulkan: Cross-platform GPU compute (Phase 2)
//   - Metal: Apple GPU (Phase 2)
//   - WebGPU: Browser/native GPU (Phase 2)
type Backend interface {
	// Element-wise binary operations
	Add(a, b *RawTensor) *RawTensor
	Sub(a, b *RawTensor) *RawTensor
	Mul(a, b *RawTensor) *RawTensor
	Div(a, b *RawTensor) *RawTensor

	// Matrix operations
	MatMul(a, b *RawTensor) *RawTensor

	// BatchMatMul performs batched matrix multiplication for 3D/4D tensors.
	// For 3D: [B, M, K] @ [B, K, N] -> [B, M, N]
	// For 4D: [B, H, M, K] @ [B, H, K, N] -> [B, H, M, N]
	BatchMatMul(a, b *RawTensor) *RawTensor

	// Convolutional operations
	Conv2D(input, kernel *RawTensor, stride, padding int) *RawTensor
	MaxPool2D(input *RawTensor, kernelSize, stride int) *RawTensor

	// Convolutional backward operations
	Conv2DInputBackward(input, kernel, grad *RawTensor, stride, padding int) *RawTensor
	Conv2DKernelBackward(input, kernel, grad *RawTensor, stride, padding int) *RawTensor
	MaxPool2DBackward(input, grad *RawTensor, maxIndices []int, kernelSize, stride int) *RawTensor

	// Shape operations
	Reshape(t *RawTensor, newShape Shape) *RawTensor
	Transpose(t *RawTensor, axes ...int) *RawTensor

	// Scalar operations (element-wise with scalar)
	MulScalar(x *RawTensor, scalar any) *RawTensor // multiply by scalar
	AddScalar(x *RawTensor, scalar any) *RawTensor // add scalar
	SubScalar(x *RawTensor, scalar any) *RawTensor // subtract scalar
	DivScalar(x *RawTensor, scalar any) *RawTensor // divide by scalar

	// Math operations (element-wise)
	Exp(x *RawTensor) *RawTensor                           // exponential
	Log(x *RawTensor) *RawTensor                           // natural logarithm
	Sqrt(x *RawTensor) *RawTensor                          // square root
	Rsqrt(x *RawTensor) *RawTensor                         // reciprocal square root (1/sqrt(x))
	Cos(x *RawTensor) *RawTensor                           // cosine
	Sin(x *RawTensor) *RawTensor                           // sine
	Erf(x *RawTensor) *RawTensor                           // error function
	Sign(x *RawTensor) *RawTensor                          // sign function
	Abs(x *RawTensor) *RawTensor                           // absolute value
	Clamp(x *RawTensor, minBound, maxBound any) *RawTensor // clamp values to [min, max]

	// Activation functions
	ReLU(x *RawTensor) *RawTensor             // max(0, x)
	Sigmoid(x *RawTensor) *RawTensor          // 1 / (1 + exp(-x))
	Tanh(x *RawTensor) *RawTensor             // hyperbolic tangent
	SiLU(x *RawTensor) *RawTensor             // x * sigmoid(x)
	Softmax(x *RawTensor, dim int) *RawTensor // softmax along dimension

	// Comparison operations (element-wise, return bool tensor)
	Greater(a, b *RawTensor) *RawTensor      // a > b
	Lower(a, b *RawTensor) *RawTensor        // a < b
	GreaterEqual(a, b *RawTensor) *RawTensor // a >= b
	LowerEqual(a, b *RawTensor) *RawTensor   // a <= b
	Equal(a, b *RawTensor) *RawTensor        // a == b
	NotEqual(a, b *RawTensor) *RawTensor     // a != b

	// Boolean operations (element-wise on bool tensors)
	Or(a, b *RawTensor) *RawTensor  // logical OR
	And(a, b *RawTensor) *RawTensor // logical AND
	Not(x *RawTensor) *RawTensor    // logical NOT

	// Reduction operations
	Sum(x *RawTensor) *RawTensor                            // total sum (scalar result)
	SumDim(x *RawTensor, dim int, keepDim bool) *RawTensor  // sum along dimension
	MeanDim(x *RawTensor, dim int, keepDim bool) *RawTensor // mean along dimension
	Argmax(x *RawTensor, dim int) *RawTensor                // index of maximum value along dimension

	// Manipulation operations
	Cat(tensors []*RawTensor, dim int) *RawTensor // concatenate along dimension
	Chunk(x *RawTensor, n, dim int) []*RawTensor  // split into n equal parts
	Unsqueeze(x *RawTensor, dim int) *RawTensor   // add dimension of size 1
	Squeeze(x *RawTensor, dim int) *RawTensor     // remove dimension of size 1

	// Indexing operations
	Gather(x *RawTensor, dim int, index *RawTensor) *RawTensor // select elements along dim using index tensor
	Where(condition, x, y *RawTensor) *RawTensor               // conditional element selection
	Embedding(weight, indices *RawTensor) *RawTensor           // lookup embeddings by indices

	// SelectAdd performs a scatter-add: accumulates src rows into dest at indexed positions.
	//
	// For dim=0 (the primary use case for Embedding backward):
	//
	//	dest: [V, D], indices: [N] (int32), src: [N, D]
	//	For each i: result[indices[i], :] += src[i, :]
	//
	// Returns a new tensor (dest is not modified in-place).
	// Semantics follow Burn's float_select_add: scatter-add along the given dimension.
	// indices must be 1-D.
	SelectAdd(dest *RawTensor, dim int, indices *RawTensor, src *RawTensor) *RawTensor

	// ScatterAdd performs a general scatter-add matching the semantics of Gather backward.
	//
	// For each flat position i in indices (same shape as src):
	//
	//	result[..., indices[...], ...] += src[...]   (the replaced axis is dim)
	//
	// This is the inverse of Gather: Gather selects elements, ScatterAdd accumulates
	// gradients back. indices has the same shape as src, with each element specifying
	// the position in dest along dim. Semantics follow Burn's float_scatter_add.
	//
	// Returns a new tensor with the same shape as dest. dest is not modified.
	// indices must have dtype int32.
	ScatterAdd(dest *RawTensor, dim int, indices *RawTensor, src *RawTensor) *RawTensor

	// Shape operations (broadcast)
	Expand(x *RawTensor, shape Shape) *RawTensor // broadcast to shape

	// Type conversion
	Cast(x *RawTensor, dtype DataType) *RawTensor // cast to different data type

	// Metadata
	Name() string
	Device() Device
}

// MemoryReclaimer is an optional interface for backends that manage GPU or
// device memory. When implemented, callers can explicitly reclaim unreferenced
// buffers at well-defined lifecycle points (e.g. after Tape.Clear) instead
// of relying on the Go garbage collector. This is critical for GPU backends
// where Go GC has no awareness of device memory pressure (ADR-015).
type MemoryReclaimer interface {
	// ReclaimMemory flushes pending GPU commands and triggers device-side
	// cleanup of buffers queued for deferred destruction. This should be
	// called after releasing a batch of intermediate tensors (e.g. at the
	// end of a training step) to ensure GPU memory is actually freed.
	ReclaimMemory()
}
