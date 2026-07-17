// Package nn provides neural network modules and layers for building deep learning models.
package nn

import (
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// FlashAttentionConfig configures the Flash Attention module.
type FlashAttentionConfig struct {
	NumHeads   int  // Number of attention heads.
	HeadDim    int  // Dimension per head.
	MaxSeqLen  int  // Maximum sequence length.
	CausalMask bool // Whether to use causal (autoregressive) masking.
	BlockSize  int  // Tile size for blocked computation (default: 64).
}

// FlashAttention implements Flash Attention 2 algorithm.
//
// Memory complexity: O(N) instead of O(N²) for standard attention.
//
// Flash Attention achieves O(N) memory by:
//  1. Tiling the computation into blocks (size B)
//  2. Processing Q blocks sequentially
//  3. For each Q block, iterating over K,V blocks
//  4. Using online softmax to accumulate results incrementally
//  5. Never materializing the full N×N attention matrix
//
// This Week 1 implementation is a CPU reference for correctness validation.
// GPU kernels will be added in later weeks.
//
// Reference: "Flash Attention 2: Faster Attention with Better Parallelism"
// Dao et al., 2023 (https://arxiv.org/abs/2307.08691)
type FlashAttention[T tensor.DType, B tensor.Backend] struct {
	config  FlashAttentionConfig
	backend B
	scale   float32 // 1/sqrt(headDim)
}

// NewFlashAttention creates a new Flash Attention module.
//
// Parameters:
//   - config: Configuration specifying heads, dimensions, block size, etc.
//   - backend: Tensor backend (CPU, GPU, etc.)
//
// Returns:
//   - *FlashAttention: Initialized Flash Attention module.
//
// Example:
//
//	config := nn.FlashAttentionConfig{
//	    NumHeads:   8,
//	    HeadDim:    64,
//	    MaxSeqLen:  2048,
//	    CausalMask: true,
//	    BlockSize:  64,
//	}
//	fa := nn.NewFlashAttention[float32](config, backend)
func NewFlashAttention[T tensor.DType, B tensor.Backend](
	config FlashAttentionConfig,
	backend B,
) *FlashAttention[T, B] {
	// Default block size if not specified
	if config.BlockSize == 0 {
		config.BlockSize = 64
	}

	scale := float32(1.0 / math.Sqrt(float64(config.HeadDim)))

	return &FlashAttention[T, B]{
		config:  config,
		backend: backend,
		scale:   scale,
	}
}

// Forward computes attention output using Flash Attention algorithm.
//
// This method implements the tiled Flash Attention algorithm:
//  1. For each query block Qi (size blockSize × headDim):
//  2. Initialize online softmax accumulator
//  3. For each key/value block (Kj, Vj):
//  4. Compute scores Sij = Qi @ Kj^T / sqrt(d)
//  5. Apply causal mask if needed
//  6. Update online softmax with (Sij, Vj)
//  7. Normalize accumulated output
//
// GPU Acceleration (Week 2): When using WebGPU backend, automatically dispatches
// to GPU-optimized Flash Attention kernel. Falls back to CPU for other backends.
//
// Parameters:
//   - q: Query tensor [batch, seqLen, numHeads, headDim]
//   - k: Key tensor [batch, kvLen, numHeads, headDim]
//   - v: Value tensor [batch, kvLen, numHeads, headDim]
//   - mask: Optional attention mask [batch, seqLen, kvLen] (currently unused, causal mask is config-driven)
//
// Returns:
//   - *tensor.Tensor: Output tensor [batch, seqLen, numHeads, headDim]
//
// Example:
//
//	Q := tensor.Randn[float32](tensor.Shape{2, 128, 8, 64}, backend)
//	K := tensor.Randn[float32](tensor.Shape{2, 128, 8, 64}, backend)
//	V := tensor.Randn[float32](tensor.Shape{2, 128, 8, 64}, backend)
//	output := fa.Forward(Q, K, V, nil)
func (fa *FlashAttention[T, B]) Forward(
	q, k, v, _ *tensor.Tensor[T, B],
) *tensor.Tensor[T, B] {
	// Validate inputs
	if len(q.Shape()) != 4 || len(k.Shape()) != 4 || len(v.Shape()) != 4 {
		panic("FlashAttention: Q, K, V must be 4D [batch, seq, numHeads, headDim]")
	}

	batch := q.Shape()[0]
	seqLen := q.Shape()[1]
	kvLen := k.Shape()[1]
	numHeads := q.Shape()[2]
	headDim := q.Shape()[3]

	if numHeads != fa.config.NumHeads || headDim != fa.config.HeadDim {
		panic("FlashAttention: numHeads or headDim mismatch with config")
	}

	// Week 2: GPU path - detect WebGPU backend and use GPU kernel
	if fa.backend.Name() == "webgpu" {
		// Type assert to WebGPU backend interface
		type webgpuBackend interface {
			FlashAttentionGPU(
				q, k, v *tensor.RawTensor,
				scale float32,
				causal bool,
				blockSize int,
			) (*tensor.RawTensor, error)
		}

		if gpuBackend, ok := any(fa.backend).(webgpuBackend); ok {
			// Execute on GPU
			outputRaw, err := gpuBackend.FlashAttentionGPU(
				q.Raw(), k.Raw(), v.Raw(),
				fa.scale,
				fa.config.CausalMask,
				fa.config.BlockSize,
			)
			if err != nil {
				panic("FlashAttention: GPU execution failed: " + err.Error())
			}

			// Wrap in typed tensor
			return tensor.New[T, B](outputRaw, fa.backend)
		}
	}

	// CPU fallback path
	qData := q.Data()
	kData := k.Data()
	vData := v.Data()

	// Convert T to float32 for CPU computation
	qFloat := convertToFloat32(qData)
	kFloat := convertToFloat32(kData)
	vFloat := convertToFloat32(vData)

	outputFloat := flashAttentionCPU(
		qFloat, kFloat, vFloat,
		batch, seqLen, kvLen, numHeads, headDim,
		fa.scale,
		fa.config.CausalMask,
		fa.config.BlockSize,
	)

	// Convert back to T and wrap in tensor
	outputData := convertFromFloat32[T](outputFloat)
	outputTensor, err := tensor.FromSlice[T](
		outputData,
		tensor.Shape{batch, seqLen, numHeads, headDim},
		fa.backend,
	)
	if err != nil {
		panic("FlashAttention: failed to create output tensor: " + err.Error())
	}

	return outputTensor
}

// FlashDims holds dimension parameters for flash attention computation.
//
// Pre-computed base offsets and strides enable bounds check elimination
// through slice pre-slicing (Burn-inspired optimization).
type FlashDims struct {
	HeadDim   int // Dimension per head.
	KVLen     int // Length of key/value sequence.
	QBase     int // Base offset for Q in flattened array.
	QStride   int // Stride between consecutive Q positions.
	KBase     int // Base offset for K in flattened array.
	KStride   int // Stride between consecutive K positions.
	VBase     int // Base offset for V in flattened array.
	VStride   int // Stride between consecutive V positions.
	OutBase   int // Base offset for output in flattened array.
	OutStride int // Stride between consecutive output positions.
}

// FlashConfig holds configuration for flash attention algorithm.
type FlashConfig struct {
	Scale     float32 // Attention scale factor (1/sqrt(headDim)).
	Causal    bool    // Whether to apply causal masking.
	BlockSize int     // Tile size for blocked computation.
}

// flashAttentionScoreBlock computes Q[i] @ K[block]^T attention scores.
//
// Designed to be inlined. Uses pre-slicing to eliminate bounds checks.
//
// Parameters:
//   - scores: Output buffer [kvBlockSize] for attention scores.
//   - q: Pre-sliced query vector [headDim].
//   - k: Full K buffer (will be sliced internally).
//   - kBase: Base offset for K in flattened array.
//   - kStride: Stride between K positions.
//   - kvStart: Start index in KV sequence.
//   - kvBlockSize: Number of KV positions in this block.
//   - headDim: Dimension per head.
//   - scale: Attention scale factor.
//   - causal: Whether to apply causal masking.
//   - queryPos: Current query position (for causal mask).
func flashAttentionScoreBlock(
	scores []float32,
	q []float32,
	k []float32,
	kBase, kStride int,
	kvStart, kvBlockSize int,
	headDim int,
	scale float32,
	causal bool,
	queryPos int,
) {
	negInf := float32(math.Inf(-1))

	for kvIdx := 0; kvIdx < kvBlockSize; kvIdx++ {
		j := kvStart + kvIdx

		// Apply causal mask: future positions get -inf.
		if causal && j > queryPos {
			scores[kvIdx] = negInf
			continue
		}

		// Pre-slice K vector for bounds check elimination.
		kOffset := kBase + j*kStride
		kVec := k[kOffset : kOffset+headDim]

		// Compute dot product Q[i] @ K[j]^T.
		var score float32
		for d := 0; d < headDim; d++ {
			score += q[d] * kVec[d]
		}
		scores[kvIdx] = score * scale
	}
}

// flashAttentionExtractValues extracts V block values.
//
// Designed to be inlined (~30 AST nodes). Uses copy() for efficient
// vectorized memory transfer.
//
// Parameters:
//   - values: Output buffer [kvBlockSize * headDim].
//   - v: Full V buffer (will be sliced internally).
//   - vBase: Base offset for V in flattened array.
//   - vStride: Stride between V positions.
//   - kvStart: Start index in KV sequence.
//   - kvBlockSize: Number of KV positions in this block.
//   - headDim: Dimension per head.
func flashAttentionExtractValues(
	values []float32,
	v []float32,
	vBase, vStride int,
	kvStart, kvBlockSize int,
	headDim int,
) {
	for kvIdx := 0; kvIdx < kvBlockSize; kvIdx++ {
		j := kvStart + kvIdx

		// Pre-slice V vector.
		vOffset := vBase + j*vStride
		vVec := v[vOffset : vOffset+headDim]

		// Copy to output buffer (vectorized by runtime).
		outOffset := kvIdx * headDim
		copy(values[outOffset:outOffset+headDim], vVec)
	}
}

// flashAttentionProcessQuery processes a single query position.
//
// Orchestrates the Flash Attention algorithm for one query:
//  1. Initialize online softmax accumulator
//  2. Iterate over KV blocks
//  3. Compute scores and extract values for each block
//  4. Update online softmax incrementally
//  5. Normalize and store final output
//
// Parameters:
//   - output: Output buffer (will be written to).
//   - q: Full Q buffer.
//   - k: Full K buffer.
//   - v: Full V buffer.
//   - queryIdx: Current query position.
//   - dims: Pre-computed dimension parameters.
//   - config: Flash Attention configuration.
func flashAttentionProcessQuery(
	output []float32,
	q, k, v []float32,
	queryIdx int,
	dims FlashDims,
	config FlashConfig,
) {
	// Initialize online softmax for this query.
	softmax := NewOnlineSoftmax(dims.HeadDim)

	// Pre-compute Q offset for this query.
	qOffset := dims.QBase + queryIdx*dims.QStride
	qVec := q[qOffset : qOffset+dims.HeadDim]

	// Iterate over KV blocks.
	for kvStart := 0; kvStart < dims.KVLen; kvStart += config.BlockSize {
		kvEnd := min(kvStart+config.BlockSize, dims.KVLen)
		kvBlockSize := kvEnd - kvStart

		// Allocate block buffers (could be pooled in future optimization).
		scores := make([]float32, kvBlockSize)
		values := make([]float32, kvBlockSize*dims.HeadDim)

		// Compute scores for this KV block.
		flashAttentionScoreBlock(
			scores, qVec, k,
			dims.KBase, dims.KStride,
			kvStart, kvBlockSize, dims.HeadDim,
			config.Scale, config.Causal, queryIdx,
		)

		// Extract V block values.
		flashAttentionExtractValues(
			values, v,
			dims.VBase, dims.VStride,
			kvStart, kvBlockSize, dims.HeadDim,
		)

		// Update online softmax with this block.
		softmax.Update(scores, values)
	}

	// Normalize and store final output.
	result := softmax.Normalize()
	outOffset := dims.OutBase + queryIdx*dims.OutStride
	copy(output[outOffset:outOffset+dims.HeadDim], result)
}

// flashAttentionCPU is the CPU reference implementation.
//
// Uses tiled computation with online softmax to achieve O(N) memory.
// Orchestrates computation by dispatching to specialized helper functions.
//
// Algorithm outline:
//
//	For each batch and head:
//	  1. Compute dimension parameters (offsets, strides)
//	  2. Process query positions in blocks
//	  3. For each query, call flashAttentionProcessQuery
//	     (which handles the tiled Flash Attention algorithm)
//
// Parameters:
//   - q: [batch * seqLen * numHeads * headDim] flattened query.
//   - k: [batch * kvLen * numHeads * headDim] flattened key.
//   - v: [batch * kvLen * numHeads * headDim] flattened value.
//   - batch, seqLen, kvLen, numHeads, headDim: Shape parameters.
//   - scale: 1/sqrt(headDim) scaling factor.
//   - causal: Whether to apply causal masking.
//   - blockSize: Tile size for blocked computation.
//
// Returns:
//   - []float32: [batch * seqLen * numHeads * headDim] flattened output.
func flashAttentionCPU(
	q, k, v []float32,
	batch, seqLen, kvLen, numHeads, headDim int,
	scale float32,
	causal bool,
	blockSize int,
) []float32 {
	output := make([]float32, batch*seqLen*numHeads*headDim)

	config := FlashConfig{
		Scale:     scale,
		Causal:    causal,
		BlockSize: blockSize,
	}

	// Compute strides between positions.
	qStride := numHeads * headDim  // Between query positions.
	kvStride := numHeads * headDim // Between KV positions.

	// Process each batch and head independently.
	for b := 0; b < batch; b++ {
		for h := 0; h < numHeads; h++ {
			// Pre-compute dimension parameters for this batch+head.
			dims := FlashDims{
				HeadDim:   headDim,
				KVLen:     kvLen,
				QBase:     b*seqLen*numHeads*headDim + h*headDim,
				QStride:   qStride,
				KBase:     b*kvLen*numHeads*headDim + h*headDim,
				KStride:   kvStride,
				VBase:     b*kvLen*numHeads*headDim + h*headDim,
				VStride:   kvStride,
				OutBase:   b*seqLen*numHeads*headDim + h*headDim,
				OutStride: qStride,
			}

			// Process query positions in blocks.
			for qStart := 0; qStart < seqLen; qStart += blockSize {
				qEnd := min(qStart+blockSize, seqLen)
				for qIdx := qStart; qIdx < qEnd; qIdx++ {
					flashAttentionProcessQuery(output, q, k, v, qIdx, dims, config)
				}
			}
		}
	}

	return output
}

// Helper functions for type conversion.

// convertToFloat32 converts generic DType slice to float32.
func convertToFloat32[T tensor.DType](data []T) []float32 {
	result := make([]float32, len(data))
	for i, v := range data {
		// Use type assertion to handle different types
		switch val := any(v).(type) {
		case float32:
			result[i] = val
		case float64:
			result[i] = float32(val)
		case int32:
			result[i] = float32(val)
		case int64:
			result[i] = float32(val)
		case uint8:
			result[i] = float32(val)
		case bool:
			if val {
				result[i] = 1.0
			} else {
				result[i] = 0.0
			}
		}
	}
	return result
}

// convertFromFloat32 converts float32 slice back to generic DType.
func convertFromFloat32[T tensor.DType](data []float32) []T {
	result := make([]T, len(data))
	var zero T
	for i, v := range data {
		// Use type assertion to handle different types
		switch any(zero).(type) {
		case float32:
			result[i] = any(v).(T)
		case float64:
			result[i] = any(float64(v)).(T)
		case int32:
			result[i] = any(int32(v)).(T)
		case int64:
			result[i] = any(int64(v)).(T)
		case uint8:
			result[i] = any(uint8(v)).(T)
		case bool:
			result[i] = any(v >= 0.5).(T)
		}
	}
	return result
}
