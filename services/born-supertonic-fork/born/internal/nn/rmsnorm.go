package nn

import (
	"github.com/born-ml/born/internal/tensor"
)

// RMSNorm applies Root Mean Square Normalization over an input tensor along the last dimension.
//
// Formula: Y = X / sqrt(mean(X^2) + eps) * gamma
//
// Where:
//   - X is the input tensor
//   - Y is the output tensor
//   - gamma is the learnable scale parameter [d_model]
//   - mean is computed along the last dimension
//   - eps is a small value to avoid division by zero
//
// RMSNorm is simpler and faster than LayerNorm (no mean subtraction),
// and is widely used in modern LLM architectures (LLaMA, Mistral, Gemma).
//
// Example:
//
//	backend := autodiff.New(cpu.New())
//	rmsnorm := nn.NewRMSNorm[AutodiffBackend](768, 1e-5, backend)
//	output := rmsnorm.Forward(hiddenStates)  // [..., 768] -> [..., 768]
type RMSNorm[B tensor.Backend] struct {
	Gamma   *Parameter[B] // learnable scale [d_model]
	Epsilon float32       // numerical stability constant
	backend B
}

// NewRMSNorm creates a new RMSNorm layer.
//
// Parameters:
//   - dModel: size of the last dimension (feature dimension)
//   - epsilon: small constant for numerical stability (typically 1e-5 or 1e-6)
//   - backend: computation backend
//
// The gamma parameter is initialized to ones.
func NewRMSNorm[B tensor.Backend](dModel int, epsilon float32, backend B) *RMSNorm[B] {
	// Initialize gamma to ones [d_model]
	gamma := tensor.Ones[float32](tensor.Shape{dModel}, backend)

	return &RMSNorm[B]{
		Gamma:   NewParameter("gamma", gamma),
		Epsilon: epsilon,
		backend: backend,
	}
}

// Forward applies RMSNorm to the input tensor.
//
// Shapes:
//   - input: [..., any, d_model]
//   - output: [..., any, d_model]
//
// Algorithm:
//  1. Compute variance = mean(x^2) along last dimension (keepdim=true)
//  2. Compute rms = sqrt(variance + epsilon)
//  3. Normalize: x_norm = x / rms
//  4. Scale: output = x_norm * gamma
func (r *RMSNorm[B]) Forward(x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// 1. Square: x^2
	xSquared := x.Mul(x)

	// 2. Mean along last dimension (keepdim=true)
	// For shape [..., d_model], we want to reduce dimension -1
	variance := xSquared.MeanDim(-1, true)

	// 3. Add epsilon for numerical stability
	epsTensor := tensor.Full[float32](variance.Shape(), r.Epsilon, r.backend)
	variancePlusEps := variance.Add(epsTensor)

	// 4. Reciprocal square root: 1 / sqrt(variance + eps)
	// We need to call Rsqrt on the backend
	rsqrtBackend, ok := any(r.backend).(interface {
		Rsqrt(*tensor.RawTensor) *tensor.RawTensor
	})
	if !ok {
		panic("RMSNorm: backend must implement Rsqrt operation")
	}
	rsqrtRaw := rsqrtBackend.Rsqrt(variancePlusEps.Raw())
	rsqrt := tensor.New[float32, B](rsqrtRaw, r.backend)

	// 5. Normalize: x * rsqrt(variance + eps)
	normalized := x.Mul(rsqrt)

	// 6. Scale by gamma
	// gamma is [d_model], need to unsqueeze to match input dimensions
	// For input [..., d_model], gamma needs to be [..., 1, ..., 1, d_model]
	// We can use broadcasting - just multiply directly
	// Broadcasting will handle [..., d_model] * [d_model] -> [..., d_model]
	gammaUnsqueezed := r.Gamma.Tensor()
	for i := 0; i < len(x.Shape())-1; i++ {
		gammaUnsqueezed = gammaUnsqueezed.Unsqueeze(0)
	}

	output := normalized.Mul(gammaUnsqueezed)

	return output
}

// Parameters returns the learnable parameters (gamma).
func (r *RMSNorm[B]) Parameters() []*Parameter[B] {
	return []*Parameter[B]{r.Gamma}
}
