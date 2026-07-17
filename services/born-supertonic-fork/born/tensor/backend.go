// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package tensor

import "github.com/born-ml/born/internal/tensor"

// Backend defines the interface that all compute backends must implement.
// Backends handle the actual computation for tensor operations.
//
// Implementations:
//   - backend/cpu: Pure Go with SIMD optimizations
//   - backend/webgpu: Cross-platform GPU compute via WebGPU
//   - backend/cuda: NVIDIA GPU via CUDA (planned)
//   - backend/vulkan: Cross-platform GPU via Vulkan (planned)
//   - backend/metal: Apple GPU via Metal (planned)
//
// Decorator backends for additional functionality:
//   - autodiff: Automatic differentiation (wraps any backend)
//
// Example:
//
//	import (
//	    "github.com/born-ml/born/tensor"
//	    "github.com/born-ml/born/backend/cpu"
//	)
//
//	backend := cpu.New()
//	x := tensor.Zeros[float32](tensor.Shape{2, 3}, backend)
//	y := tensor.Ones[float32](tensor.Shape{2, 3}, backend)
//	z := x.Add(y)  // Uses backend.Add under the hood
type Backend interface {
	// Element-wise binary operations.
	Add(a, b *RawTensor) *RawTensor // Element-wise addition.
	Sub(a, b *RawTensor) *RawTensor // Element-wise subtraction.
	Mul(a, b *RawTensor) *RawTensor // Element-wise multiplication.
	Div(a, b *RawTensor) *RawTensor // Element-wise division.

	// Matrix operations.
	MatMul(a, b *RawTensor) *RawTensor      // Matrix multiplication.
	BatchMatMul(a, b *RawTensor) *RawTensor // Batched matrix multiplication for 3D/4D tensors.

	// Convolutional operations.
	Conv2D(input, kernel *RawTensor, stride, padding int) *RawTensor                               // 2D convolution.
	MaxPool2D(input *RawTensor, kernelSize, stride int) *RawTensor                                 // 2D max pooling.
	Conv2DInputBackward(input, kernel, grad *RawTensor, stride, padding int) *RawTensor            // Conv2D input gradient.
	Conv2DKernelBackward(input, kernel, grad *RawTensor, stride, padding int) *RawTensor           // Conv2D kernel gradient.
	MaxPool2DBackward(input, grad *RawTensor, maxIndices []int, kernelSize, stride int) *RawTensor // MaxPool2D gradient.

	// Shape operations.
	Reshape(t *RawTensor, newShape Shape) *RawTensor // Reshape tensor.
	Transpose(t *RawTensor, axes ...int) *RawTensor  // Transpose dimensions.

	// Scalar operations (element-wise with scalar).
	MulScalar(x *RawTensor, scalar any) *RawTensor // Multiply by scalar.
	AddScalar(x *RawTensor, scalar any) *RawTensor // Add scalar.
	SubScalar(x *RawTensor, scalar any) *RawTensor // Subtract scalar.
	DivScalar(x *RawTensor, scalar any) *RawTensor // Divide by scalar.

	// Math operations (element-wise).
	Exp(x *RawTensor) *RawTensor                           // Exponential.
	Log(x *RawTensor) *RawTensor                           // Natural logarithm.
	Sqrt(x *RawTensor) *RawTensor                          // Square root.
	Rsqrt(x *RawTensor) *RawTensor                         // Reciprocal square root (1/sqrt(x)).
	Cos(x *RawTensor) *RawTensor                           // Cosine.
	Sin(x *RawTensor) *RawTensor                           // Sine.
	Erf(x *RawTensor) *RawTensor                           // Error function (erf).
	Sign(x *RawTensor) *RawTensor                          // Sign function.
	Abs(x *RawTensor) *RawTensor                           // Absolute value.
	Clamp(x *RawTensor, minBound, maxBound any) *RawTensor // Clamp values to [min, max].

	// Activation functions.
	ReLU(x *RawTensor) *RawTensor             // ReLU: max(0, x).
	Sigmoid(x *RawTensor) *RawTensor          // Sigmoid: 1 / (1 + exp(-x)).
	Tanh(x *RawTensor) *RawTensor             // Hyperbolic tangent.
	SiLU(x *RawTensor) *RawTensor             // SiLU (Swish): x * sigmoid(x).
	Softmax(x *RawTensor, dim int) *RawTensor // Softmax along dimension.

	// Comparison operations (element-wise, return bool tensor).
	Greater(a, b *RawTensor) *RawTensor      // a > b.
	Lower(a, b *RawTensor) *RawTensor        // a < b.
	GreaterEqual(a, b *RawTensor) *RawTensor // a >= b.
	LowerEqual(a, b *RawTensor) *RawTensor   // a <= b.
	Equal(a, b *RawTensor) *RawTensor        // a == b.
	NotEqual(a, b *RawTensor) *RawTensor     // a != b.

	// Boolean operations (element-wise on bool tensors).
	Or(a, b *RawTensor) *RawTensor  // Logical OR.
	And(a, b *RawTensor) *RawTensor // Logical AND.
	Not(x *RawTensor) *RawTensor    // Logical NOT.

	// Reduction operations.
	Sum(x *RawTensor) *RawTensor                            // Total sum (scalar result).
	SumDim(x *RawTensor, dim int, keepDim bool) *RawTensor  // Sum along dimension.
	MeanDim(x *RawTensor, dim int, keepDim bool) *RawTensor // Mean along dimension.
	Argmax(x *RawTensor, dim int) *RawTensor                // Index of maximum value along dimension.

	// Manipulation operations.
	Cat(tensors []*RawTensor, dim int) *RawTensor // Concatenate along dimension.
	Chunk(x *RawTensor, n, dim int) []*RawTensor  // Split into n equal parts.
	Unsqueeze(x *RawTensor, dim int) *RawTensor   // Add dimension of size 1.
	Squeeze(x *RawTensor, dim int) *RawTensor     // Remove dimension of size 1.

	// Indexing operations.
	Gather(x *RawTensor, dim int, index *RawTensor) *RawTensor                          // Select elements along dim using index tensor.
	Where(condition, x, y *RawTensor) *RawTensor                                        // Conditional element selection.
	Embedding(weight, indices *RawTensor) *RawTensor                                    // Lookup embeddings by indices.
	SelectAdd(dest *RawTensor, dim int, indices *RawTensor, src *RawTensor) *RawTensor  // Scatter-add (1-D indices): dest[indices[i], ...] += src[i, ...].
	ScatterAdd(dest *RawTensor, dim int, indices *RawTensor, src *RawTensor) *RawTensor // Scatter-add (N-D indices, Gather backward): dest[..., indices[...], ...] += src[...].

	// Shape operations (broadcast).
	Expand(x *RawTensor, shape Shape) *RawTensor // Broadcast to shape.

	// Type conversion.
	Cast(x *RawTensor, dtype DataType) *RawTensor // Cast to different data type.

	// Metadata.
	Name() string   // Backend name (e.g., "CPU", "WebGPU").
	Device() Device // Device type.
}

// Compile-time check that internal Backend implements public Backend.
var _ Backend = tensor.Backend(nil)
