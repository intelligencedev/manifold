package nn

import (
	"github.com/born-ml/born/internal/tensor"
)

// LayerNorm applies Layer Normalization over an input tensor along the last dimension.
//
// Formula: Y = gamma * (X - mean(X)) / sqrt(var(X) + eps) + beta
//
// Where:
//   - X is the input tensor
//   - Y is the output tensor
//   - gamma is the learnable scale parameter [d_model]
//   - beta is the learnable shift parameter [d_model]
//   - mean and variance are computed along the last dimension
//   - eps is a small value to avoid division by zero
//
// LayerNorm normalizes activations by computing statistics across features,
// which helps stabilize training and is widely used in transformers (BERT, GPT, etc.).
//
// Example:
//
//	backend := autodiff.New(cpu.New())
//	layernorm := nn.NewLayerNorm[AutodiffBackend](768, 1e-5, backend)
//	output := layernorm.Forward(hiddenStates)  // [..., 768] -> [..., 768]
type LayerNorm[B tensor.Backend] struct {
	Gamma   *Parameter[B] // learnable scale [d_model]
	Beta    *Parameter[B] // learnable shift [d_model]
	Epsilon float32       // numerical stability constant
	backend B
}

// NewLayerNorm creates a new LayerNorm layer.
//
// Parameters:
//   - normalizedShape: size of the last dimension (feature dimension)
//   - epsilon: small constant for numerical stability (typically 1e-5 or 1e-6)
//   - backend: computation backend
//
// The gamma parameter is initialized to ones, beta to zeros.
func NewLayerNorm[B tensor.Backend](normalizedShape int, epsilon float32, backend B) *LayerNorm[B] {
	// Initialize gamma to ones [normalized_shape]
	gamma := tensor.Ones[float32](tensor.Shape{normalizedShape}, backend)

	// Initialize beta to zeros [normalized_shape]
	beta := tensor.Zeros[float32](tensor.Shape{normalizedShape}, backend)

	return &LayerNorm[B]{
		Gamma:   NewParameter("gamma", gamma),
		Beta:    NewParameter("beta", beta),
		Epsilon: epsilon,
		backend: backend,
	}
}

// Forward applies LayerNorm to the input tensor.
//
// Shapes:
//   - input: [..., any, d_model]
//   - output: [..., any, d_model]
//
// Algorithm:
//  1. Compute mean = mean(x) along last dimension (keepdim=true)
//  2. Subtract mean: x_centered = x - mean
//  3. Compute variance = mean((x - mean)^2) along last dimension
//  4. Normalize: x_norm = x_centered / sqrt(variance + epsilon)
//  5. Scale and shift: output = gamma * x_norm + beta
func (l *LayerNorm[B]) Forward(x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// 1. Compute mean along last dimension (keepdim=true)
	mean := x.MeanDim(-1, true)

	// 2. Subtract mean: x_centered = x - mean
	xCentered := x.Sub(mean)

	// 3. Compute variance: var = mean((x - mean)^2)
	variance := xCentered.Mul(xCentered).MeanDim(-1, true)

	// 4. Add epsilon for numerical stability
	epsTensor := tensor.Full[float32](variance.Shape(), l.Epsilon, l.backend)
	variancePlusEps := variance.Add(epsTensor)

	// 5. Reciprocal square root: 1 / sqrt(variance + eps)
	rsqrtBackend, ok := any(l.backend).(interface {
		Rsqrt(*tensor.RawTensor) *tensor.RawTensor
	})
	if !ok {
		panic("LayerNorm: backend must implement Rsqrt operation")
	}
	rsqrtRaw := rsqrtBackend.Rsqrt(variancePlusEps.Raw())
	rsqrt := tensor.New[float32, B](rsqrtRaw, l.backend)

	// 6. Normalize: x_norm = x_centered * rsqrt(variance + eps)
	xNorm := xCentered.Mul(rsqrt)

	// 7. Scale and shift: output = gamma * x_norm + beta
	// gamma and beta are [d_model], need to unsqueeze to match input dimensions
	// For input [..., d_model], gamma/beta need to be [..., 1, ..., 1, d_model]
	// Broadcasting will handle [..., d_model] * [d_model] -> [..., d_model]
	gammaUnsqueezed := l.Gamma.Tensor()
	betaUnsqueezed := l.Beta.Tensor()

	for i := 0; i < len(x.Shape())-1; i++ {
		gammaUnsqueezed = gammaUnsqueezed.Unsqueeze(0)
		betaUnsqueezed = betaUnsqueezed.Unsqueeze(0)
	}

	output := xNorm.Mul(gammaUnsqueezed).Add(betaUnsqueezed)

	return output
}

// Parameters returns the learnable parameters (gamma and beta).
func (l *LayerNorm[B]) Parameters() []*Parameter[B] {
	return []*Parameter[B]{l.Gamma, l.Beta}
}
