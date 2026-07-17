package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// LinearOption is a functional option for configuring a Linear layer.
type LinearOption func(*linearConfig)

// linearConfig holds configuration for Linear layer creation.
type linearConfig struct {
	useBias bool
}

// defaultLinearConfig returns the default configuration.
func defaultLinearConfig() linearConfig {
	return linearConfig{
		useBias: true, // Default: use bias (backwards compatible)
	}
}

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
	return func(cfg *linearConfig) {
		cfg.useBias = useBias
	}
}

// Linear implements a fully connected (dense) layer.
//
// Performs the transformation: y = x @ W.T + b
// where:
//   - x is the input tensor with shape [batch_size, in_features]
//   - W is the weight matrix with shape [out_features, in_features]
//   - b is the bias vector with shape [out_features] (optional, see WithBias)
//   - y is the output tensor with shape [batch_size, out_features]
//
// Weights are initialized using Xavier/Glorot initialization.
// Biases are initialized to zeros (if enabled).
//
// Example:
//
//	backend := cpu.New()
//	layer := nn.NewLinear(784, 128, backend)
//
//	input := tensor.Randn[float32](tensor.Shape{32, 784}, backend)  // batch_size=32
//	output := layer.Forward(input)  // shape: [32, 128]
//
//	// Without bias (for LLaMA-style models)
//	lm_head := nn.NewLinear(512, vocab_size, backend, nn.WithBias(false))
type Linear[B tensor.Backend] struct {
	inFeatures  int
	outFeatures int
	weight      *Parameter[B] // [out_features, in_features]
	bias        *Parameter[B] // [out_features]
	backend     B
}

// NewLinear creates a new Linear layer.
//
// Weights are initialized using Xavier/Glorot uniform distribution.
// Biases are initialized to zeros (if enabled).
//
// Parameters:
//   - inFeatures: Number of input features
//   - outFeatures: Number of output features
//   - backend: Backend to use for tensor operations
//   - opts: Optional configuration (see WithBias)
//
// Returns a new Linear layer.
//
// Example:
//
//	// With bias (default)
//	layer := nn.NewLinear(784, 128, backend)
//
//	// Without bias (for LLaMA, attention projections, etc.)
//	lm_head := nn.NewLinear(hidden_size, vocab_size, backend, nn.WithBias(false))
func NewLinear[B tensor.Backend](inFeatures, outFeatures int, backend B, opts ...LinearOption) *Linear[B] {
	// Apply options
	cfg := defaultLinearConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	// Weight: [out_features, in_features]
	weightShape := tensor.Shape{outFeatures, inFeatures}
	weightTensor := Xavier(inFeatures, outFeatures, weightShape, backend)
	weight := NewParameter("weight", weightTensor)

	// Bias: [out_features] (optional)
	var bias *Parameter[B]
	if cfg.useBias {
		biasShape := tensor.Shape{outFeatures}
		biasTensor := Zeros(biasShape, backend)
		bias = NewParameter("bias", biasTensor)
	}

	return &Linear[B]{
		inFeatures:  inFeatures,
		outFeatures: outFeatures,
		weight:      weight,
		bias:        bias,
		backend:     backend,
	}
}

// Forward computes the output of the linear layer.
//
// Performs: y = x @ W.T + b
//
// Input shape: [batch_size, in_features]
// Output shape: [batch_size, out_features]
//
// Parameters:
//   - input: Input tensor with shape [batch_size, in_features]
//
// Returns output tensor with shape [batch_size, out_features].
func (l *Linear[B]) Forward(input *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// Validate input shape
	inputShape := input.Shape()
	if len(inputShape) != 2 {
		panic(fmt.Sprintf("Linear.Forward: expected 2D input [batch, features], got shape %v", inputShape))
	}
	if inputShape[1] != l.inFeatures {
		panic(fmt.Sprintf("Linear.Forward: expected input with %d features, got %d", l.inFeatures, inputShape[1]))
	}

	// Get weight tensor
	w := l.weight.Tensor() // [out_features, in_features]

	// Transpose weight: W.T has shape [in_features, out_features]
	wT := w.Transpose() // [in_features, out_features]

	// Matrix multiplication: x @ W.T
	// [batch_size, in_features] @ [in_features, out_features] = [batch_size, out_features]
	output := input.MatMul(wT)

	// Add bias if present
	if l.bias != nil {
		b := l.bias.Tensor() // [out_features]
		// Broadcast bias: b has shape [out_features], broadcast to [batch_size, out_features]
		// We need to reshape bias to [1, out_features] for proper broadcasting
		bReshaped := b.Reshape(1, l.outFeatures)
		output = output.Add(bReshaped)
	}

	return output
}

// Parameters returns the trainable parameters of this layer.
//
// Returns [weight, bias] if bias is present, otherwise [weight].
func (l *Linear[B]) Parameters() []*Parameter[B] {
	if l.bias != nil {
		return []*Parameter[B]{l.weight, l.bias}
	}
	return []*Parameter[B]{l.weight}
}

// Weight returns the weight parameter.
func (l *Linear[B]) Weight() *Parameter[B] {
	return l.weight
}

// Bias returns the bias parameter.
func (l *Linear[B]) Bias() *Parameter[B] {
	return l.bias
}

// InFeatures returns the number of input features.
func (l *Linear[B]) InFeatures() int {
	return l.inFeatures
}

// OutFeatures returns the number of output features.
func (l *Linear[B]) OutFeatures() int {
	return l.outFeatures
}

// HasBias returns true if this layer has a bias parameter.
func (l *Linear[B]) HasBias() bool {
	return l.bias != nil
}

// StateDict returns a map of parameter names to raw tensors.
func (l *Linear[B]) StateDict() map[string]*tensor.RawTensor {
	stateDict := make(map[string]*tensor.RawTensor)
	stateDict["weight"] = l.weight.Tensor().Raw()
	if l.bias != nil {
		stateDict["bias"] = l.bias.Tensor().Raw()
	}
	return stateDict
}

// LoadStateDict loads parameters from a state dictionary.
func (l *Linear[B]) LoadStateDict(stateDict map[string]*tensor.RawTensor) error {
	// Load weight
	weightRaw, ok := stateDict["weight"]
	if !ok {
		return fmt.Errorf("missing weight in state dict")
	}

	// Validate weight shape
	expectedWeightShape := tensor.Shape{l.outFeatures, l.inFeatures}
	if !weightRaw.Shape().Equal(expectedWeightShape) {
		return fmt.Errorf("weight shape mismatch: expected %v, got %v",
			expectedWeightShape, weightRaw.Shape())
	}

	// Validate weight dtype
	if weightRaw.DType() != tensor.Float32 {
		return fmt.Errorf("weight dtype mismatch: expected float32, got %v",
			weightRaw.DType())
	}

	// Copy weight data
	weightData := l.weight.Tensor().Data()
	copy(weightData, weightRaw.AsFloat32())

	// Load bias if present
	if l.bias != nil {
		biasRaw, ok := stateDict["bias"]
		if !ok {
			return fmt.Errorf("missing bias in state dict")
		}

		// Validate bias shape
		expectedBiasShape := tensor.Shape{l.outFeatures}
		if !biasRaw.Shape().Equal(expectedBiasShape) {
			return fmt.Errorf("bias shape mismatch: expected %v, got %v",
				expectedBiasShape, biasRaw.Shape())
		}

		// Validate bias dtype
		if biasRaw.DType() != tensor.Float32 {
			return fmt.Errorf("bias dtype mismatch: expected float32, got %v",
				biasRaw.DType())
		}

		// Copy bias data
		biasData := l.bias.Tensor().Data()
		copy(biasData, biasRaw.AsFloat32())
	}

	return nil
}
