// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

// Package nn provides neural network modules and layers for the Born ML Framework.
//
// This package offers building blocks for constructing neural networks:
//   - Module interface: Base interface for all NN components
//   - Parameter interface: Trainable parameters with gradient tracking
//   - Linear: Fully connected layer
//   - Conv2D: Convolutional layer
//   - Activations: ReLU, Sigmoid, Tanh, SiLU
//   - Normalization: RMSNorm, LayerNorm
//   - Embedding: Token embeddings
//   - Attention: Multi-head attention, causal masking
//   - Loss functions: MSE, CrossEntropy
//   - Sequential: Container for stacking layers
//
// Design inspired by PyTorch's nn.Module but adapted for Go generics and type safety.
package nn

import (
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// NewParameter creates a new trainable parameter.
//
// The parameter tensor should be initialized before creating the Parameter.
// Gradient will be allocated during the first backward pass.
//
// Parameters:
//   - name: Descriptive name for this parameter (e.g., "linear1.weight")
//   - t: The initialized parameter tensor
//
// Returns a new Parameter.
//
// Example:
//
//	backend := cpu.New()
//	weights := tensor.Randn[float32](tensor.Shape{128, 784}, backend)
//	param := nn.NewParameter("layer1.weight", weights)
func NewParameter[B tensor.Backend](name string, t *tensor.Tensor[float32, B]) *Parameter[B] {
	return nn.NewParameter(name, t)
}

// Reproducibility

// SetSeed sets the global random seed for weight initialization and tensor creation.
//
// Call this before creating models to ensure reproducible initialization across runs.
// Seeds both nn (Xavier, Embedding) and tensor (Randn, Rand) random sources.
//
// Example:
//
//	nn.SetSeed(42)
//	model1 := nn.NewLinear(784, 128, backend)
//
//	nn.SetSeed(42)
//	model2 := nn.NewLinear(784, 128, backend) // identical weights
func SetSeed(seed int64) {
	nn.SetSeed(seed)
}

// ResetSeed clears the seeded RNG, reverting to Go's default auto-seeded behavior.
func ResetSeed() {
	nn.ResetSeed()
}

// Layers

// Linear represents a fully connected (dense) layer.
type Linear[B tensor.Backend] = nn.Linear[B]

// LinearOption is a functional option for configuring a Linear layer.
type LinearOption = nn.LinearOption

// WithBias sets whether the Linear layer should use bias.
//
// Default is true. Set to false for architectures like LLaMA that don't use bias.
//
// Example:
//
//	// Linear layer without bias (LLaMA-style)
//	lm_head := nn.NewLinear(hidden_size, vocab_size, backend, nn.WithBias(false))
//
//	// Linear layer with bias (default)
//	layer := nn.NewLinear(784, 128, backend)  // same as WithBias(true)
func WithBias(useBias bool) LinearOption {
	return nn.WithBias(useBias)
}

// NewLinear creates a new linear layer with Xavier initialization.
//
// Example:
//
//	backend := cpu.New()
//	layer := nn.NewLinear(784, 128, backend)
//
//	// Without bias (for LLaMA, attention projections, etc.)
//	lm_head := nn.NewLinear(hidden_size, vocab_size, backend, nn.WithBias(false))
func NewLinear[B tensor.Backend](inFeatures, outFeatures int, backend B, opts ...LinearOption) *Linear[B] {
	return nn.NewLinear(inFeatures, outFeatures, backend, opts...)
}

// Conv2D represents a 2D convolutional layer.
type Conv2D[B tensor.Backend] = nn.Conv2D[B]

// NewConv2D creates a new 2D convolutional layer.
//
// Example:
//
//	backend := cpu.New()
//	conv := nn.NewConv2D(1, 32, 3, 3, 1, 1, true, backend)  // in_channels=1, out_channels=32, kernel=3x3, stride=1, padding=1, useBias=true
func NewConv2D[B tensor.Backend](
	inChannels, outChannels int,
	kernelH, kernelW int,
	stride, padding int,
	useBias bool,
	backend B,
) *Conv2D[B] {
	return nn.NewConv2D(inChannels, outChannels, kernelH, kernelW, stride, padding, useBias, backend)
}

// MaxPool2D represents a 2D max pooling layer.
type MaxPool2D[B tensor.Backend] = nn.MaxPool2D[B]

// NewMaxPool2D creates a new 2D max pooling layer.
//
// Example:
//
//	backend := cpu.New()
//	pool := nn.NewMaxPool2D(2, 2, backend)  // kernel=2, stride=2
func NewMaxPool2D[B tensor.Backend](kernelSize, stride int, backend B) *MaxPool2D[B] {
	return nn.NewMaxPool2D(kernelSize, stride, backend)
}

// Activations

// ReLU represents the Rectified Linear Unit activation function.
type ReLU[B tensor.Backend] = nn.ReLU[B]

// NewReLU creates a new ReLU activation layer.
//
// Example:
//
//	relu := nn.NewReLU()
func NewReLU[B tensor.Backend]() *ReLU[B] {
	return nn.NewReLU[B]()
}

// Sigmoid represents the Sigmoid activation function.
type Sigmoid[B tensor.Backend] = nn.Sigmoid[B]

// NewSigmoid creates a new Sigmoid activation layer.
//
// Example:
//
//	sigmoid := nn.NewSigmoid()
func NewSigmoid[B tensor.Backend]() *Sigmoid[B] {
	return nn.NewSigmoid[B]()
}

// Tanh represents the Tanh activation function.
type Tanh[B tensor.Backend] = nn.Tanh[B]

// NewTanh creates a new Tanh activation layer.
//
// Example:
//
//	tanh := nn.NewTanh()
func NewTanh[B tensor.Backend]() *Tanh[B] {
	return nn.NewTanh[B]()
}

// SiLU represents the Sigmoid Linear Unit (SiLU/Swish) activation function.
// SiLU(x) = x * sigmoid(x).
type SiLU[B tensor.Backend] = nn.SiLU[B]

// NewSiLU creates a new SiLU activation layer.
//
// Example:
//
//	silu := nn.NewSiLU[B]()
//	output := silu.Forward(input)
func NewSiLU[B tensor.Backend]() *SiLU[B] {
	return nn.NewSiLU[B]()
}

// Embedding and Normalization Layers

// Embedding represents a lookup table for embeddings.
type Embedding[B tensor.Backend] = nn.Embedding[B]

// NewEmbedding creates a new embedding layer.
//
// Example:
//
//	backend := cpu.New()
//	embed := nn.NewEmbedding[B](50000, 768, backend)  // vocab=50000, dim=768
//	tokenIds := tensor.FromSlice([]int32{1, 5, 10}, tensor.Shape{1, 3}, backend)
//	embeddings := embed.Forward(tokenIds)  // [1, 3, 768]
func NewEmbedding[B tensor.Backend](numEmbeddings, embeddingDim int, backend B) *Embedding[B] {
	return nn.NewEmbedding(numEmbeddings, embeddingDim, backend)
}

// RMSNorm represents Root Mean Square Layer Normalization.
type RMSNorm[B tensor.Backend] = nn.RMSNorm[B]

// NewRMSNorm creates a new RMSNorm layer.
//
// Example:
//
//	backend := cpu.New()
//	norm := nn.NewRMSNorm[B](768, 1e-5, backend)
//	output := norm.Forward(input)  // [..., 768] -> [..., 768]
func NewRMSNorm[B tensor.Backend](dModel int, epsilon float32, backend B) *RMSNorm[B] {
	return nn.NewRMSNorm(dModel, epsilon, backend)
}

// LayerNorm represents Layer Normalization.
type LayerNorm[B tensor.Backend] = nn.LayerNorm[B]

// NewLayerNorm creates a new LayerNorm layer.
//
// Example:
//
//	backend := cpu.New()
//	norm := nn.NewLayerNorm[B](768, 1e-5, backend)
//	output := norm.Forward(input)  // [..., 768] -> [..., 768]
func NewLayerNorm[B tensor.Backend](normalizedShape int, epsilon float32, backend B) *LayerNorm[B] {
	return nn.NewLayerNorm(normalizedShape, epsilon, backend)
}

// Loss Functions

// CrossEntropyLoss represents the cross-entropy loss for classification.
type CrossEntropyLoss[B tensor.Backend] = nn.CrossEntropyLoss[B]

// NewCrossEntropyLoss creates a new cross-entropy loss function.
//
// Example:
//
//	backend := cpu.New()
//	criterion := nn.NewCrossEntropyLoss(backend)
//	loss := criterion.Forward(logits, labels)
func NewCrossEntropyLoss[B tensor.Backend](backend B) *CrossEntropyLoss[B] {
	return nn.NewCrossEntropyLoss(backend)
}

// MSELoss represents the mean squared error loss for regression.
type MSELoss[B tensor.Backend] = nn.MSELoss[B]

// NewMSELoss creates a new MSE loss function.
//
// Example:
//
//	backend := cpu.New()
//	criterion := nn.NewMSELoss(backend)
//	loss := criterion.Forward(predictions, targets)
func NewMSELoss[B tensor.Backend](backend B) *MSELoss[B] {
	return nn.NewMSELoss(backend)
}

// Sequential

// Sequential represents a sequential container of modules.
type Sequential[B tensor.Backend] = nn.Sequential[B]

// NewSequential creates a new sequential model.
//
// Example:
//
//	backend := cpu.New()
//	model := nn.NewSequential(
//	    nn.NewLinear(784, 128, backend),
//	    nn.NewReLU(),
//	    nn.NewLinear(128, 10, backend),
//	)
func NewSequential[B tensor.Backend](modules ...Module[B]) *Sequential[B] {
	// Convert public Module[B] slice to internal Module[B] slice.
	// Go interfaces with same methods are not directly assignable.
	internalModules := make([]nn.Module[B], len(modules))
	for i, m := range modules {
		internalModules[i] = m
	}
	return nn.NewSequential(internalModules...)
}

// Initialization functions

// Xavier initializes a tensor using Xavier/Glorot initialization.
//
// Example:
//
//	backend := cpu.New()
//	weights := nn.Xavier(784, 128, tensor.Shape{128, 784}, backend)
func Xavier[B tensor.Backend](fanIn, fanOut int, shape tensor.Shape, backend B) *tensor.Tensor[float32, B] {
	return nn.Xavier(fanIn, fanOut, shape, backend)
}

// Zeros initializes a tensor with zeros (for biases).
//
// Example:
//
//	backend := cpu.New()
//	bias := nn.Zeros(tensor.Shape{128}, backend)
func Zeros[B tensor.Backend](shape tensor.Shape, backend B) *tensor.Tensor[float32, B] {
	return nn.Zeros(shape, backend)
}

// Ones initializes a tensor with ones.
//
// Example:
//
//	backend := cpu.New()
//	weights := nn.Ones(tensor.Shape{128, 784}, backend)
func Ones[B tensor.Backend](shape tensor.Shape, backend B) *tensor.Tensor[float32, B] {
	return nn.Ones(shape, backend)
}

// Randn initializes a tensor with random values from N(0, 1).
//
// Example:
//
//	backend := cpu.New()
//	weights := nn.Randn(tensor.Shape{128, 784}, backend)
func Randn[B tensor.Backend](shape tensor.Shape, backend B) *tensor.Tensor[float32, B] {
	return nn.Randn(shape, backend)
}

// Attention Functions

// ScaledDotProductAttention computes attention scores using the scaled dot-product mechanism.
//
// This is the core attention mechanism used in transformers.
//
// Parameters:
//   - query: Query tensor [batch, heads, seq_q, head_dim]
//   - key: Key tensor [batch, heads, seq_k, head_dim]
//   - value: Value tensor [batch, heads, seq_k, head_dim]
//   - mask: Optional attention mask [batch, 1, seq_q, seq_k] or nil (additive mask, -inf for masked)
//   - scale: Scaling factor (0 for auto-compute as 1/sqrt(head_dim))
//
// Returns:
//   - output: Attended values [batch, heads, seq_q, head_dim]
//   - weights: Attention weights [batch, heads, seq_q, seq_k]
//
// Example:
//
//	Q := tensor.Randn[float32](tensor.Shape{2, 8, 10, 64}, backend)
//	K := tensor.Randn[float32](tensor.Shape{2, 8, 10, 64}, backend)
//	V := tensor.Randn[float32](tensor.Shape{2, 8, 10, 64}, backend)
//	output, weights := nn.ScaledDotProductAttention(Q, K, V, nil, 0)
func ScaledDotProductAttention[B tensor.Backend](
	query, key, value *tensor.Tensor[float32, B],
	mask *tensor.Tensor[float32, B],
	scale float32,
) (*tensor.Tensor[float32, B], *tensor.Tensor[float32, B]) {
	return nn.ScaledDotProductAttention(query, key, value, mask, scale)
}

// CausalMask creates a causal (autoregressive) attention mask.
//
// In causal attention, each position can only attend to earlier positions.
// This is used in autoregressive models like GPT.
//
// Returns a mask tensor where future positions are masked with -inf.
// Shape: [1, 1, seq_len, seq_len] (broadcastable to [batch, heads, seq, seq])
//
// Example:
//
//	mask := nn.CausalMask(10, backend)  // [1, 1, 10, 10]
//	output, weights := nn.ScaledDotProductAttention(Q, K, V, mask, 0)
func CausalMask[B tensor.Backend](seqLen int, backend B) *tensor.Tensor[float32, B] {
	return nn.CausalMask(seqLen, backend)
}

// MultiHeadAttention represents the multi-head attention mechanism.
type MultiHeadAttention[B tensor.Backend] = nn.MultiHeadAttention[B]

// NewMultiHeadAttention creates a new multi-head attention module.
//
// Parameters:
//   - embedDim: Total embedding dimension (must be divisible by numHeads)
//   - numHeads: Number of attention heads
//   - backend: Computation backend
//
// Example:
//
//	backend := cpu.New()
//	mha := nn.NewMultiHeadAttention[B](768, 12, backend)  // BERT-base config
//	output := mha.Forward(x, x, x, nil)  // Self-attention
func NewMultiHeadAttention[B tensor.Backend](embedDim, numHeads int, backend B) *MultiHeadAttention[B] {
	return nn.NewMultiHeadAttention(embedDim, numHeads, backend)
}

// Utility functions

// CrossEntropyBackward computes the backward pass for cross-entropy loss.
func CrossEntropyBackward[B tensor.Backend](
	logits *tensor.Tensor[float32, B],
	targets *tensor.Tensor[int32, B],
	backend B,
) *tensor.Tensor[float32, B] {
	return nn.CrossEntropyBackward(logits, targets, backend)
}

// Accuracy computes the classification accuracy.
//
// Example:
//
//	acc := nn.Accuracy(predictions, labels)
//	fmt.Printf("Accuracy: %.2f%%\n", acc*100)
func Accuracy[B tensor.Backend](
	logits *tensor.Tensor[float32, B],
	targets *tensor.Tensor[int32, B],
) float32 {
	return nn.Accuracy(logits, targets)
}

// NewEmbeddingWithWeight creates an embedding layer from an existing weight tensor.
//
// This is useful when loading pre-trained embeddings.
//
// Example:
//
//	weights := tensor.Randn[float32](tensor.Shape{50000, 768}, backend)
//	embed := nn.NewEmbeddingWithWeight(weights)
func NewEmbeddingWithWeight[B tensor.Backend](weight *tensor.Tensor[float32, B]) *Embedding[B] {
	return nn.NewEmbeddingWithWeight(weight)
}

// ReLUFunc applies the ReLU activation function element-wise.
// ReLU(x) = max(0, x).
func ReLUFunc[B tensor.Backend](x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	return nn.ReLUFunc(x)
}

// SigmoidFunc applies the sigmoid activation function element-wise.
// Sigmoid(x) = 1 / (1 + exp(-x)).
func SigmoidFunc[B tensor.Backend](x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	return nn.SigmoidFunc(x)
}
