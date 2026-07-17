// Package autodiff implements automatic differentiation using the decorator pattern.
//
// AutodiffBackend wraps any Backend implementation (CPU, GPU, etc.) and adds
// gradient tracking capabilities through a GradientTape.
//
// Architecture:
//   - Decorator pattern: AutodiffBackend[B] wraps any Backend implementation
//   - GradientTape: Records operations during forward pass
//   - Operation interface: Each op (Add, Mul, MatMul) implements backward pass
//   - Reverse-mode AD: Computes gradients efficiently using chain rule
//
// Usage:
//
//	// Wrap any backend with autodiff
//	cpuBackend := cpu.New()
//	autodiffBackend := autodiff.New(cpuBackend)
//
//	// Use with tensors
//	x := tensor.FromSlice([]float32{2.0}, tensor.Shape{1}, autodiffBackend)
//	y := x.Mul(x) // y = x²
//
//	// Compute gradients
//	y.Backward()
//	fmt.Println(x.Grad()) // dy/dx = 2x = 4.0
package autodiff

import (
	"fmt"

	"github.com/born-ml/born/internal/autodiff/ops"
	"github.com/born-ml/born/internal/tensor"
)

// AutodiffBackend wraps a Backend and adds automatic differentiation.
// It implements the tensor.Backend interface and records operations in a GradientTape.
//
// Type parameter B must satisfy the tensor.Backend interface.
type AutodiffBackend[B tensor.Backend] struct {
	inner B             // Wrapped backend (CPU, GPU, etc.)
	tape  *GradientTape // Records operations for backpropagation
}

// New creates a new AutodiffBackend wrapping the given backend.
func New[B tensor.Backend](backend B) *AutodiffBackend[B] {
	return &AutodiffBackend[B]{
		inner: backend,
		tape:  NewGradientTape(),
	}
}

// Tape returns the gradient tape for manual control.
// Useful for:
//   - Starting/stopping recording
//   - Clearing tape between iterations
//   - Inspecting recorded operations
func (b *AutodiffBackend[B]) Tape() *GradientTape {
	return b.tape
}

// ClearTape clears the gradient tape and reclaims GPU memory from intermediate
// tensors. This is the recommended way to clear the tape between training steps
// instead of calling Tape().Clear() directly. It releases all intermediate GPU
// buffers recorded during the forward pass, then flushes the GPU command queue
// and triggers device-side destruction of deferred buffers (ADR-015).
func (b *AutodiffBackend[B]) ClearTape() {
	b.tape.Clear()
	// Flush deferred GPU buffer releases to pool. tape.Clear() calls
	// DeferReleaseGPUBuffer which queues buffers in activeBatch — without
	// flushing, they stay there and are never returned to pool for reuse.
	// Use FlushGPU (not ReclaimMemory) to avoid killing carry state tensors.
	if flusher, ok := any(b.inner).(interface{ FlushGPU() }); ok {
		flusher.FlushGPU()
	}
}

// Inner returns the wrapped backend for direct access.
func (b *AutodiffBackend[B]) Inner() B {
	return b.inner
}

// Name returns the backend name.
func (b *AutodiffBackend[B]) Name() string {
	return "Autodiff(" + b.inner.Name() + ")"
}

// Device returns the compute device.
func (b *AutodiffBackend[B]) Device() tensor.Device {
	return b.inner.Device()
}

// Add performs element-wise addition and records the operation.
func (b *AutodiffBackend[B]) Add(a, c *tensor.RawTensor) *tensor.RawTensor {
	// CRITICAL: Prevent inplace modification that would corrupt autodiff graph.
	// Temporarily increase refCount so IsUnique() returns false.
	// This forces CPU backend to allocate new result instead of inplace modification.
	defer a.ForceNonUnique()()
	defer c.ForceNonUnique()()

	// Forward pass using wrapped backend
	result := b.inner.Add(a, c)

	// Record operation if tape is recording
	if b.tape.IsRecording() {
		op := ops.NewAddOp(a, c, result)
		b.tape.Record(op)
	}

	return result
}

// Sub performs element-wise subtraction and records the operation.
func (b *AutodiffBackend[B]) Sub(a, c *tensor.RawTensor) *tensor.RawTensor {
	defer a.ForceNonUnique()()
	defer c.ForceNonUnique()()

	result := b.inner.Sub(a, c)

	if b.tape.IsRecording() {
		op := ops.NewSubOp(a, c, result)
		b.tape.Record(op)
	}

	return result
}

// Mul performs element-wise multiplication and records the operation.
func (b *AutodiffBackend[B]) Mul(a, c *tensor.RawTensor) *tensor.RawTensor {
	defer a.ForceNonUnique()()
	defer c.ForceNonUnique()()

	result := b.inner.Mul(a, c)

	if b.tape.IsRecording() {
		op := ops.NewMulOp(a, c, result)
		b.tape.Record(op)
	}

	return result
}

// Div performs element-wise division and records the operation.
func (b *AutodiffBackend[B]) Div(a, c *tensor.RawTensor) *tensor.RawTensor {
	defer a.ForceNonUnique()()
	defer c.ForceNonUnique()()

	result := b.inner.Div(a, c)

	if b.tape.IsRecording() {
		op := ops.NewDivOp(a, c, result)
		b.tape.Record(op)
	}

	return result
}

// MatMul performs matrix multiplication and records the operation.
func (b *AutodiffBackend[B]) MatMul(a, c *tensor.RawTensor) *tensor.RawTensor {
	defer a.ForceNonUnique()()
	defer c.ForceNonUnique()()

	result := b.inner.MatMul(a, c)

	if b.tape.IsRecording() {
		op := ops.NewMatMulOp(a, c, result)
		b.tape.Record(op)
	}

	return result
}

// BatchMatMul performs batched matrix multiplication and records the operation.
func (b *AutodiffBackend[B]) BatchMatMul(a, c *tensor.RawTensor) *tensor.RawTensor {
	defer a.ForceNonUnique()()
	defer c.ForceNonUnique()()

	result := b.inner.BatchMatMul(a, c)

	if b.tape.IsRecording() {
		op := ops.NewBatchMatMulOp(a, c, result)
		b.tape.Record(op)
	}

	return result
}

// Reshape reshapes a tensor and records the operation.
//
// CRITICAL: Like Transpose, Reshape must be recorded on tape!
// Without recording, gradients won't flow back to reshaped parameters.
//
// Example: Conv2D bias
//   - bias parameter: [out_channels]
//   - reshaped for broadcasting: [1, out_channels, 1, 1]
//   - Without ReshapeOp: gradient computed for reshaped tensor only
//   - With ReshapeOp: gradient propagates back to original bias parameter
func (b *AutodiffBackend[B]) Reshape(t *tensor.RawTensor, newShape tensor.Shape) *tensor.RawTensor {
	defer t.ForceNonUnique()()

	// Forward pass using wrapped backend
	result := b.inner.Reshape(t, newShape)

	// Record operation if tape is recording
	if b.tape.IsRecording() {
		op := ops.NewReshapeOp(t, result)
		b.tape.Record(op)
	}

	return result
}

// Transpose transposes a tensor and records the operation.
//
// CRITICAL: Even though conceptually transpose is a "view", the underlying
// backend may create a new tensor (e.g., CPU backend copies data).
// We MUST record this operation so gradients flow back correctly.
//
// For example, in Linear layer:
//
//	w = weight parameter
//	wT = w.Transpose()  // Creates NEW tensor!
//	output = input @ wT  // MatMul records operation with wT
//
// Without recording Transpose:
//   - Backward computes grad for wT (new tensor)
//   - Optimizer looks for grad of w (original parameter)
//   - NO GRADIENT FOUND! Parameters don't update!
//
// With TransposeOp:
//   - Backward computes grad for wT
//   - TransposeOp.Backward propagates grad back to w
//   - Optimizer finds grad for w ✓
func (b *AutodiffBackend[B]) Transpose(t *tensor.RawTensor, axes ...int) *tensor.RawTensor {
	defer t.ForceNonUnique()()

	// Handle default axes (reverse all dimensions)
	ndim := len(t.Shape())
	if len(axes) == 0 {
		axes = make([]int, ndim)
		for i := range axes {
			axes[i] = ndim - 1 - i
		}
	}

	// Forward pass using wrapped backend
	result := b.inner.Transpose(t, axes...)

	// Record operation if tape is recording
	if b.tape.IsRecording() {
		op := ops.NewTransposeOp(t, result, axes)
		b.tape.Record(op)
	}

	return result
}

// Conv2D performs 2D convolution and records the operation.
//
// CRITICAL: Conv2D must be recorded on tape for gradient flow!
// Just like Transpose, Conv2D creates new tensors and without recording,
// gradients won't flow back to the kernel/input parameters.
func (b *AutodiffBackend[B]) Conv2D(input, kernel *tensor.RawTensor, stride, padding int) *tensor.RawTensor {
	defer input.ForceNonUnique()()
	defer kernel.ForceNonUnique()()

	// Forward pass using wrapped backend
	result := b.inner.Conv2D(input, kernel, stride, padding)

	// Record operation if tape is recording
	if b.tape.IsRecording() {
		op := ops.NewConv2DOp(input, kernel, result, stride, padding)
		b.tape.Record(op)
	}

	return result
}

// MaxPool2D performs 2D max pooling and records the operation.
//
// CRITICAL: MaxPool2D must be recorded on tape for gradient flow!
// During backward pass, gradients only flow to positions that had max values.
// MaxPool2DOp stores max indices during forward pass for correct gradient routing.
func (b *AutodiffBackend[B]) MaxPool2D(input *tensor.RawTensor, kernelSize, stride int) *tensor.RawTensor {
	defer input.ForceNonUnique()()

	// Forward pass using wrapped backend
	result := b.inner.MaxPool2D(input, kernelSize, stride)

	// Record operation if tape is recording
	if b.tape.IsRecording() {
		op := ops.NewMaxPool2DOp(input, result, kernelSize, stride)
		b.tape.Record(op)
	}

	return result
}

// ReLU applies ReLU activation and records the operation.
// Delegates to inner backend (GPU shader in LazyMode) instead of CPU loop.
func (b *AutodiffBackend[B]) ReLU(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.ReLU(x)

	if b.tape.IsRecording() {
		op := ops.NewReLUOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Sigmoid applies sigmoid activation: σ(x) = 1 / (1 + exp(-x)).
// Delegates to inner backend (GPU shader in LazyMode) instead of CPU loop.
func (b *AutodiffBackend[B]) Sigmoid(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Sigmoid(x)

	if b.tape.IsRecording() {
		op := ops.NewSigmoidOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Tanh applies hyperbolic tangent activation.
// Delegates to inner backend (GPU shader in LazyMode) instead of CPU loop.
func (b *AutodiffBackend[B]) Tanh(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Tanh(x)

	if b.tape.IsRecording() {
		op := ops.NewTanhOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// SiLU applies SiLU (Swish) activation: f(x) = x * sigmoid(x).
// Delegates to inner backend (GPU shader in LazyMode) instead of CPU loop.
func (b *AutodiffBackend[B]) SiLU(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.SiLU(x)

	if b.tape.IsRecording() {
		op := ops.NewSiLUOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Log computes element-wise natural logarithm.
// Delegates to inner backend (GPU shader in LazyMode) instead of CPU loop.
func (b *AutodiffBackend[B]) Log(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Log(x)

	if b.tape.IsRecording() {
		op := ops.NewLogOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Softmax applies softmax activation along the specified dimension.
//
// Parameters:
//   - x: Input tensor
//   - dim: Dimension along which to compute softmax (-1 for last dimension)
//
// Forward (for each row):
//
//	softmax(x)_i = exp(x_i - max(x)) / Σ_j exp(x_j - max(x))
//
// The max-shifting ensures numerical stability (prevents overflow).
//
// Backward:
//
//	The Jacobian of softmax is complex, but the gradient simplifies to:
//	∂L/∂x_j = softmax_j * (∂L/∂softmax_j - Σ_i (∂L/∂softmax_i * softmax_i))
func (b *AutodiffBackend[B]) Softmax(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	// Forward pass using wrapped backend
	result := b.inner.Softmax(x, dim)

	// Record operation if tape is recording
	if b.tape.IsRecording() {
		op := ops.NewSoftmaxOp(x, result, dim)
		b.tape.Record(op)
	}

	return result
}

// CrossEntropy computes cross-entropy loss for classification.
//
// Forward:
//
//	Loss = mean(-log_softmax(logits)[targets])
//
// Uses the log-sum-exp trick for numerical stability.
//
// Backward:
//
//	∂L/∂logits = (softmax(logits) - y_one_hot) / batch_size
//
// Parameters:
//   - logits: Model predictions [batch_size, num_classes]
//   - targets: Ground truth class indices [batch_size]
//
// Returns:
//   - Scalar loss value (mean over batch)
func (b *AutodiffBackend[B]) CrossEntropy(logits, targets *tensor.RawTensor) *tensor.RawTensor {
	defer logits.ForceNonUnique()()

	// Forward: -mean(log_softmax(logits)[targets]) via backend ops composition.
	// All ops stay on GPU — no CPU readback of logits/targets.
	//
	// Step 1: log_softmax = log(softmax(logits))
	softmax := b.inner.Softmax(logits, -1) // [batch, classes] — GPU
	logSoftmax := b.inner.Log(softmax)     // [batch, classes] — GPU

	// Step 2: gather log-probs at target indices → [batch, 1]
	targetsUnsqueezed := b.inner.Unsqueeze(targets, -1)          // [batch, 1]
	targetsCast := b.inner.Cast(targetsUnsqueezed, tensor.Int32) // ensure int32
	logProbs := b.inner.Gather(logSoftmax, 1, targetsCast)       // [batch, 1]

	// Step 3: mean(-log_probs) → [1] loss tensor
	negLogProbs := b.inner.MulScalar(logProbs, typedNeg1(logits.DType())) // [batch, 1]
	result := b.inner.MeanDim(negLogProbs, 0, true)                       // [1, 1]
	result = b.inner.Reshape(result, tensor.Shape{1})                     // [1]

	if b.tape.IsRecording() {
		op := ops.NewCrossEntropyOp(logits, targets, result)
		b.tape.Record(op)
	}

	return result
}

func typedNeg1(dtype tensor.DataType) any {
	if dtype == tensor.Float64 {
		return float64(-1)
	}
	return float32(-1)
}

// Exp computes element-wise exponential and records the operation.
func (b *AutodiffBackend[B]) Exp(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Exp(x)

	if b.tape.IsRecording() {
		op := ops.NewExpOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Erf computes element-wise error function and records the operation.
func (b *AutodiffBackend[B]) Erf(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Erf(x)

	if b.tape.IsRecording() {
		op := ops.NewErfOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Sqrt computes element-wise square root and records the operation.
func (b *AutodiffBackend[B]) Sqrt(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Sqrt(x)

	if b.tape.IsRecording() {
		op := ops.NewSqrtOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Rsqrt computes element-wise reciprocal square root and records the operation.
func (b *AutodiffBackend[B]) Rsqrt(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Rsqrt(x)

	if b.tape.IsRecording() {
		op := ops.NewRsqrtOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Cos computes element-wise cosine and records the operation.
func (b *AutodiffBackend[B]) Cos(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Cos(x)

	if b.tape.IsRecording() {
		op := ops.NewCosOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Sin computes element-wise sine and records the operation.
func (b *AutodiffBackend[B]) Sin(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Sin(x)

	if b.tape.IsRecording() {
		op := ops.NewSinOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Sign computes element-wise sign and records the operation.
func (b *AutodiffBackend[B]) Sign(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Sign(x)

	if b.tape.IsRecording() {
		op := ops.NewSignOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// Abs computes element-wise absolute value and records the operation.
func (b *AutodiffBackend[B]) Abs(x *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.Abs(x)

	if b.tape.IsRecording() {
		op := ops.NewAbsOp(x, result)
		b.tape.Record(op)
	}

	return result
}

// SumDim sums tensor along a dimension and records the operation.
func (b *AutodiffBackend[B]) SumDim(x *tensor.RawTensor, dim int, keepDim bool) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.SumDim(x, dim, keepDim)

	if b.tape.IsRecording() {
		op := ops.NewSumDimOp(x, result, dim, keepDim)
		b.tape.Record(op)
	}

	return result
}

// MeanDim computes mean along a dimension and records the operation.
func (b *AutodiffBackend[B]) MeanDim(x *tensor.RawTensor, dim int, keepDim bool) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	result := b.inner.MeanDim(x, dim, keepDim)

	if b.tape.IsRecording() {
		op := ops.NewMeanDimOp(x, result, dim, keepDim)
		b.tape.Record(op)
	}

	return result
}

// NoGrad temporarily disables gradient recording for inference.
//
// This is useful for:
//   - Inference/evaluation (no need to track gradients)
//   - Gradient-free operations (e.g., updating exponential moving averages)
//   - Memory optimization (gradient tape doesn't grow)
//
// The function executes the provided function with gradient recording disabled,
// then restores the previous recording state.
//
// Example:
//
//	// Inference mode
//	backend.NoGrad(func() {
//	    output := model.Forward(input)  // No gradients recorded
//	    predictions := output.ArgMax()
//	})
//
//	// Training continues normally
//	loss := model.Forward(trainInput)
//	loss.Backward()  // Gradients computed
func (b *AutodiffBackend[B]) NoGrad(fn func()) {
	wasRecording := b.tape.IsRecording()
	b.tape.StopRecording()
	defer func() {
		if wasRecording {
			b.tape.StartRecording()
		}
	}()
	fn()
}

// MulScalar multiplies tensor elements by a scalar and records the operation.
func (b *AutodiffBackend[B]) MulScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	result := b.inner.MulScalar(x, scalar)
	if b.tape.IsRecording() {
		b.tape.Record(ops.NewMulScalarOp(x, result, scalar))
	}
	return result
}

// AddScalar adds a scalar to tensor elements and records the operation.
func (b *AutodiffBackend[B]) AddScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	result := b.inner.AddScalar(x, scalar)
	if b.tape.IsRecording() {
		b.tape.Record(ops.NewAddScalarOp(x, result))
	}
	return result
}

// SubScalar subtracts a scalar from tensor elements and records the operation.
func (b *AutodiffBackend[B]) SubScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	result := b.inner.SubScalar(x, scalar)
	if b.tape.IsRecording() {
		b.tape.Record(ops.NewSubScalarOp(x, result))
	}
	return result
}

// DivScalar divides tensor elements by a scalar and records the operation.
func (b *AutodiffBackend[B]) DivScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	result := b.inner.DivScalar(x, scalar)
	if b.tape.IsRecording() {
		b.tape.Record(ops.NewDivScalarOp(x, result, scalar))
	}
	return result
}

// Greater performs element-wise greater-than comparison (autodiff proxy).
func (b *AutodiffBackend[B]) Greater(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.Greater(a, other)
}

// Lower performs element-wise less-than comparison (autodiff proxy).
func (b *AutodiffBackend[B]) Lower(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.Lower(a, other)
}

// GreaterEqual performs element-wise greater-or-equal comparison (autodiff proxy).
func (b *AutodiffBackend[B]) GreaterEqual(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.GreaterEqual(a, other)
}

// LowerEqual performs element-wise less-or-equal comparison (autodiff proxy).
func (b *AutodiffBackend[B]) LowerEqual(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.LowerEqual(a, other)
}

// Equal performs element-wise equality comparison (autodiff proxy).
func (b *AutodiffBackend[B]) Equal(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.Equal(a, other)
}

// NotEqual performs element-wise inequality comparison (autodiff proxy).
func (b *AutodiffBackend[B]) NotEqual(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.NotEqual(a, other)
}

// Or performs element-wise logical OR (autodiff proxy).
func (b *AutodiffBackend[B]) Or(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.Or(a, other)
}

// And performs element-wise logical AND (autodiff proxy).
func (b *AutodiffBackend[B]) And(a, other *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.And(a, other)
}

// Not performs element-wise logical NOT (autodiff proxy).
func (b *AutodiffBackend[B]) Not(x *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.Not(x)
}

// Sum reduces tensor to a single scalar by summing all elements (autodiff proxy).
func (b *AutodiffBackend[B]) Sum(x *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.Sum(x)
}

// Argmax returns indices of maximum values along a dimension (autodiff proxy).
func (b *AutodiffBackend[B]) Argmax(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	return b.inner.Argmax(x, dim)
}

// Expand broadcasts tensor to a larger shape (autodiff proxy).
func (b *AutodiffBackend[B]) Expand(x *tensor.RawTensor, shape tensor.Shape) *tensor.RawTensor {
	return b.inner.Expand(x, shape)
}

// Cast converts tensor to a different data type (autodiff proxy).
func (b *AutodiffBackend[B]) Cast(x *tensor.RawTensor, dtype tensor.DataType) *tensor.RawTensor {
	return b.inner.Cast(x, dtype)
}

// Cat concatenates tensors along a dimension.
func (b *AutodiffBackend[B]) Cat(tensors []*tensor.RawTensor, dim int) *tensor.RawTensor {
	// Mark all inputs as non-unique for safety
	//nolint:gocritic // defer in loop is intentional for cleanup of all inputs
	for _, t := range tensors {
		defer t.ForceNonUnique()()
	}

	// Perform forward pass
	result := b.inner.Cat(tensors, dim)

	// Record operation for gradient computation
	if b.tape.IsRecording() {
		// Normalize dimension and compute sizes
		ndim := len(tensors[0].Shape())
		normalizedDim := dim
		if normalizedDim < 0 {
			normalizedDim = ndim + normalizedDim
		}

		sizes := make([]int, len(tensors))
		for i, t := range tensors {
			sizes[i] = t.Shape()[normalizedDim]
		}

		op := ops.NewCatOp(tensors, normalizedDim, sizes, result)
		b.tape.Record(op)
	}

	return result
}

// Chunk splits tensor into equal parts.
// Note: Multi-output operation - gradient computation requires special handling.
func (b *AutodiffBackend[B]) Chunk(x *tensor.RawTensor, n, dim int) []*tensor.RawTensor {
	defer x.ForceNonUnique()()

	// Perform forward pass
	results := b.inner.Chunk(x, n, dim)

	// Record operation for gradient computation
	// Note: ChunkOp is a multi-output operation.
	// The tape needs to handle this specially since the Operation interface
	// expects a single output.
	if b.tape.IsRecording() {
		// Normalize dimension
		ndim := len(x.Shape())
		normalizedDim := dim
		if normalizedDim < 0 {
			normalizedDim = ndim + normalizedDim
		}

		op := ops.NewChunkOp(x, n, normalizedDim, results)
		b.tape.Record(op)
	}

	return results
}

// Unsqueeze adds a dimension of size 1 at the specified position.
// Recorded via Reshape for proper gradient flow.
func (b *AutodiffBackend[B]) Unsqueeze(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	oldShape := x.Shape()
	ndim := len(oldShape)

	// Normalize negative dimension (for unsqueeze, we can insert at ndim+1 positions)
	if dim < 0 {
		dim = ndim + 1 + dim
	}
	if dim < 0 || dim > ndim {
		panic(fmt.Sprintf("unsqueeze: dim %d out of range for %dD tensor", dim, ndim))
	}

	// Compute new shape with 1 at position dim
	newShape := make(tensor.Shape, ndim+1)
	for i := 0; i < dim; i++ {
		newShape[i] = oldShape[i]
	}
	newShape[dim] = 1
	for i := dim; i < ndim; i++ {
		newShape[i+1] = oldShape[i]
	}

	// Use Reshape which IS recorded on tape
	return b.Reshape(x, newShape)
}

// Squeeze removes dimension of size 1 at the specified position.
// Recorded via Reshape for proper gradient flow.
func (b *AutodiffBackend[B]) Squeeze(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	oldShape := x.Shape()
	ndim := len(oldShape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("squeeze: dim %d out of range for %dD tensor", dim, ndim))
	}

	// Only squeeze if dimension is size 1
	if oldShape[dim] != 1 {
		// Can't squeeze dimension that's not size 1, return as-is (no-op reshape)
		return b.Reshape(x, oldShape)
	}

	// Compute new shape without the squeezed dimension
	newShape := make(tensor.Shape, 0, ndim-1)
	for i := 0; i < ndim; i++ {
		if i != dim {
			newShape = append(newShape, oldShape[i])
		}
	}

	// Handle empty shape case (scalar result)
	if len(newShape) == 0 {
		newShape = tensor.Shape{1}
	}

	// Use Reshape which IS recorded on tape
	return b.Reshape(x, newShape)
}

// Gather selects elements along dim using index tensor.
func (b *AutodiffBackend[B]) Gather(x *tensor.RawTensor, dim int, index *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()
	defer index.ForceNonUnique()()

	// Perform forward pass
	result := b.inner.Gather(x, dim, index)

	// Record operation for gradient computation
	if b.tape.IsRecording() {
		// Normalize dimension
		ndim := len(x.Shape())
		normalizedDim := dim
		if normalizedDim < 0 {
			normalizedDim = ndim + normalizedDim
		}

		op := ops.NewGatherOp(x, normalizedDim, index, result)
		b.tape.Record(op)
	}

	return result
}

// Clamp restricts tensor values element-wise to [minBound, maxBound].
func (b *AutodiffBackend[B]) Clamp(x *tensor.RawTensor, minBound, maxBound any) *tensor.RawTensor {
	defer x.ForceNonUnique()()

	// Perform forward pass
	result := b.inner.Clamp(x, minBound, maxBound)

	// Record operation for gradient computation
	if b.tape.IsRecording() {
		op := ops.NewClampOp(x, minBound, maxBound, result)
		b.tape.Record(op)
	}

	return result
}

// Where performs conditional element selection.
//
// output[i] = x[i] if condition[i] else y[i]
//
// Backward:
//
//	grad_x = where(cond, grad_out, 0)
//	grad_y = where(cond, 0, grad_out)
func (b *AutodiffBackend[B]) Where(condition, x, y *tensor.RawTensor) *tensor.RawTensor {
	defer x.ForceNonUnique()()
	defer y.ForceNonUnique()()

	// Forward pass
	result := b.inner.Where(condition, x, y)

	// Record operation for gradient computation
	if b.tape.IsRecording() {
		op := ops.NewWhereOp(condition, x, y, result)
		b.tape.Record(op)
	}

	return result
}

// Embedding performs embedding lookup with autodiff support.
//
// weight: [numEmbeddings, embeddingDim]
// indices: any shape of int32 indices
// output: [...indices.shape, embeddingDim]
//
// The gradient for weight is computed via scatter-add.
func (b *AutodiffBackend[B]) Embedding(weight, indices *tensor.RawTensor) *tensor.RawTensor {
	defer weight.ForceNonUnique()()

	// Forward pass
	result := b.inner.Embedding(weight, indices)

	// Record operation for gradient computation
	if b.tape.IsRecording() {
		op := ops.NewEmbeddingOp(weight, indices, result)
		b.tape.Record(op)
	}

	return result
}

// SelectAdd performs a scatter-add along the specified dimension (autodiff proxy).
//
// SelectAdd is used only inside backward passes (e.g., Embedding backward) and
// does not need to be recorded on the tape: it computes gradients, not forward
// values. Delegating directly to the inner backend mirrors the pattern used for
// Conv2DInputBackward and MaxPool2DBackward.
func (b *AutodiffBackend[B]) SelectAdd(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.SelectAdd(dest, dim, indices, src)
}

// ScatterAdd performs a general scatter-add matching Gather backward semantics (autodiff proxy).
//
// ScatterAdd is used only inside backward passes (e.g., Gather backward) and
// does not need to be recorded on the tape: it computes gradients, not forward
// values. Delegating directly to the inner backend mirrors the pattern used for
// SelectAdd and Conv2DInputBackward.
func (b *AutodiffBackend[B]) ScatterAdd(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) *tensor.RawTensor {
	return b.inner.ScatterAdd(dest, dim, indices, src)
}

// Conv2DInputBackward computes gradient w.r.t. input for Conv2D.
// Delegates to inner backend (no recording needed - used during backward pass only).
func (b *AutodiffBackend[B]) Conv2DInputBackward(input, kernel, grad *tensor.RawTensor, stride, padding int) *tensor.RawTensor {
	return b.inner.Conv2DInputBackward(input, kernel, grad, stride, padding)
}

// Conv2DKernelBackward computes gradient w.r.t. kernel for Conv2D.
// Delegates to inner backend (no recording needed - used during backward pass only).
func (b *AutodiffBackend[B]) Conv2DKernelBackward(input, kernel, grad *tensor.RawTensor, stride, padding int) *tensor.RawTensor {
	return b.inner.Conv2DKernelBackward(input, kernel, grad, stride, padding)
}

// MaxPool2DBackward computes gradient w.r.t. input for MaxPool2D.
// Delegates to inner backend (no recording needed - used during backward pass only).
func (b *AutodiffBackend[B]) MaxPool2DBackward(input, grad *tensor.RawTensor, maxIndices []int, kernelSize, stride int) *tensor.RawTensor {
	return b.inner.MaxPool2DBackward(input, grad, maxIndices, kernelSize, stride)
}

// ReclaimMemory proxies to inner backend's MemoryReclaimer if available.
func (b *AutodiffBackend[B]) ReclaimMemory() {
	if reclaimer, ok := any(b.inner).(tensor.MemoryReclaimer); ok {
		reclaimer.ReclaimMemory()
	}
}
