package nn

import (
	"github.com/born-ml/born/internal/tensor"
)

// ReLUBackend is an interface for backends that support ReLU activation.
type ReLUBackend interface {
	ReLU(*tensor.RawTensor) *tensor.RawTensor
}

// SigmoidBackend is an interface for backends that support Sigmoid activation.
type SigmoidBackend interface {
	Sigmoid(*tensor.RawTensor) *tensor.RawTensor
}

// TanhBackend is an interface for backends that support Tanh activation.
type TanhBackend interface {
	Tanh(*tensor.RawTensor) *tensor.RawTensor
}

// ReLU is a Rectified Linear Unit activation module.
//
// Applies the element-wise function: f(x) = max(0, x)
//
// ReLU is the most commonly used activation function in deep learning.
// It helps with the vanishing gradient problem and is computationally efficient.
//
// Example:
//
//	relu := nn.NewReLU[Backend]()
//	output := relu.Forward(input)  // All negative values become 0
type ReLU[B tensor.Backend] struct{}

// NewReLU creates a new ReLU activation module.
func NewReLU[B tensor.Backend]() *ReLU[B] {
	return &ReLU[B]{}
}

// Forward applies ReLU activation: f(x) = max(0, x).
func (r *ReLU[B]) Forward(input *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	backend := input.Backend()

	// Check if backend supports ReLU via interface
	if reluBackend, ok := any(backend).(ReLUBackend); ok {
		resultRaw := reluBackend.ReLU(input.Raw())
		return tensor.New[float32, B](resultRaw, backend)
	}

	// Fallback: backend doesn't support ReLU
	panic("ReLU: backend must implement ReLU operation (use autodiff.AutodiffBackend)")
}

// Parameters returns an empty slice (ReLU has no trainable parameters).
func (r *ReLU[B]) Parameters() []*Parameter[B] {
	return nil
}

// StateDict returns an empty map (ReLU has no trainable parameters).
func (r *ReLU[B]) StateDict() map[string]*tensor.RawTensor {
	return make(map[string]*tensor.RawTensor)
}

// LoadStateDict loads parameters (no-op for ReLU).
func (r *ReLU[B]) LoadStateDict(_ map[string]*tensor.RawTensor) error {
	return nil
}

// Sigmoid is a sigmoid activation module.
//
// Applies the element-wise function: σ(x) = 1 / (1 + exp(-x))
//
// Sigmoid squashes values to the range (0, 1), making it useful for
// binary classification and gate mechanisms in LSTMs/GRUs.
//
// Example:
//
//	sigmoid := nn.NewSigmoid[Backend]()
//	output := sigmoid.Forward(input)  // Values in range (0, 1)
type Sigmoid[B tensor.Backend] struct{}

// NewSigmoid creates a new Sigmoid activation module.
func NewSigmoid[B tensor.Backend]() *Sigmoid[B] {
	return &Sigmoid[B]{}
}

// Forward applies Sigmoid activation: σ(x) = 1 / (1 + exp(-x)).
func (s *Sigmoid[B]) Forward(input *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	backend := input.Backend()

	// Check if backend supports Sigmoid via interface
	if sigmoidBackend, ok := any(backend).(SigmoidBackend); ok {
		resultRaw := sigmoidBackend.Sigmoid(input.Raw())
		return tensor.New[float32, B](resultRaw, backend)
	}

	// Fallback
	panic("Sigmoid: backend must implement Sigmoid operation (use autodiff.AutodiffBackend)")
}

// Parameters returns an empty slice (Sigmoid has no trainable parameters).
func (s *Sigmoid[B]) Parameters() []*Parameter[B] {
	return nil
}

// StateDict returns an empty map (Sigmoid has no trainable parameters).
func (s *Sigmoid[B]) StateDict() map[string]*tensor.RawTensor {
	return make(map[string]*tensor.RawTensor)
}

// LoadStateDict loads parameters (no-op for Sigmoid).
func (s *Sigmoid[B]) LoadStateDict(_ map[string]*tensor.RawTensor) error {
	return nil
}

// Tanh is a hyperbolic tangent activation module.
//
// Applies the element-wise function: tanh(x) = (exp(x) - exp(-x)) / (exp(x) + exp(-x))
//
// Tanh squashes values to the range (-1, 1), making it zero-centered
// which can help with training. Often used in RNNs.
//
// Example:
//
//	tanh := nn.NewTanh[Backend]()
//	output := tanh.Forward(input)  // Values in range (-1, 1)
type Tanh[B tensor.Backend] struct{}

// NewTanh creates a new Tanh activation module.
func NewTanh[B tensor.Backend]() *Tanh[B] {
	return &Tanh[B]{}
}

// Forward applies Tanh activation.
func (t *Tanh[B]) Forward(input *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	backend := input.Backend()

	// Check if backend supports Tanh via interface
	if tanhBackend, ok := any(backend).(TanhBackend); ok {
		resultRaw := tanhBackend.Tanh(input.Raw())
		return tensor.New[float32, B](resultRaw, backend)
	}

	// Fallback
	panic("Tanh: backend must implement Tanh operation (use autodiff.AutodiffBackend)")
}

// Parameters returns an empty slice (Tanh has no trainable parameters).
func (t *Tanh[B]) Parameters() []*Parameter[B] {
	return nil
}

// StateDict returns an empty map (Tanh has no trainable parameters).
func (t *Tanh[B]) StateDict() map[string]*tensor.RawTensor {
	return make(map[string]*tensor.RawTensor)
}

// LoadStateDict loads parameters (no-op for Tanh).
func (t *Tanh[B]) LoadStateDict(_ map[string]*tensor.RawTensor) error {
	return nil
}

// SiLUBackend is an interface for backends that support SiLU activation.
type SiLUBackend interface {
	SiLU(*tensor.RawTensor) *tensor.RawTensor
}

// SiLU is a SiLU (Swish) activation module.
//
// Applies the element-wise function: f(x) = x * sigmoid(x)
//
// SiLU (Sigmoid Linear Unit), also known as Swish, is widely used in
// modern transformer architectures like LLaMA, Mistral, and GPT-Neo.
// It provides smooth, non-monotonic activation that helps with gradient flow.
//
// Example:
//
//	silu := nn.NewSiLU[Backend]()
//	output := silu.Forward(input)  // Smooth activation
type SiLU[B tensor.Backend] struct{}

// NewSiLU creates a new SiLU activation module.
func NewSiLU[B tensor.Backend]() *SiLU[B] {
	return &SiLU[B]{}
}

// Forward applies SiLU activation: f(x) = x * sigmoid(x).
func (s *SiLU[B]) Forward(input *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	backend := input.Backend()

	// Check if backend supports SiLU via interface
	if siluBackend, ok := any(backend).(SiLUBackend); ok {
		resultRaw := siluBackend.SiLU(input.Raw())
		return tensor.New[float32, B](resultRaw, backend)
	}

	// Fallback
	panic("SiLU: backend must implement SiLU operation (use autodiff.AutodiffBackend)")
}

// Parameters returns an empty slice (SiLU has no trainable parameters).
func (s *SiLU[B]) Parameters() []*Parameter[B] {
	return nil
}

// StateDict returns an empty map (SiLU has no trainable parameters).
func (s *SiLU[B]) StateDict() map[string]*tensor.RawTensor {
	return make(map[string]*tensor.RawTensor)
}

// LoadStateDict loads parameters (no-op for SiLU).
func (s *SiLU[B]) LoadStateDict(_ map[string]*tensor.RawTensor) error {
	return nil
}
