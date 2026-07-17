package nn

import (
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// SiLUFunc applies SiLU (Swish) activation: f(x) = x * sigmoid(x).
//
// This is the functional version of SiLU activation, useful in GLU variants.
//
// Example:
//
//	output := nn.SiLUFunc(input)
func SiLUFunc[B tensor.Backend](x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	backend := x.Backend()

	// Check if backend supports SiLU via interface
	if siluBackend, ok := any(backend).(SiLUBackend); ok {
		resultRaw := siluBackend.SiLU(x.Raw())
		return tensor.New[float32, B](resultRaw, backend)
	}

	panic("SiLUFunc: backend must implement SiLU operation (use autodiff.AutodiffBackend)")
}

// GELUFunc applies GELU (Gaussian Error Linear Unit) activation.
//
// Uses the tanh approximation: 0.5 * x * (1 + tanh(sqrt(2/pi) * (x + 0.044715 * x^3))).
//
// GELU is used in BERT, GPT-2, and other transformers.
//
// Example:
//
//	output := nn.GELUFunc(input)
func GELUFunc[B tensor.Backend](x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// Fallback: compute using tanh approximation
	// GELU(x) ≈ 0.5 * x * (1 + tanh(sqrt(2/pi) * (x + 0.044715 * x^3)))
	return geluTanhApprox(x)
}

// geluTanhApprox computes GELU using tanh approximation.
func geluTanhApprox[B tensor.Backend](x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	backend := x.Backend()

	// Constants
	sqrt2pi := float32(math.Sqrt(2.0 / math.Pi)) // ~0.7978845608
	c := float32(0.044715)

	// x^3
	x3 := x.Mul(x).Mul(x)

	// x + 0.044715 * x^3
	inner := x.Add(x3.MulScalar(c))

	// sqrt(2/pi) * (x + 0.044715 * x^3)
	inner = inner.MulScalar(sqrt2pi)

	// tanh(...) via backend
	if tanhBackend, ok := any(backend).(TanhBackend); ok {
		tanhRaw := tanhBackend.Tanh(inner.Raw())
		tanhResult := tensor.New[float32, B](tanhRaw, backend)

		// 1 + tanh(...)
		onePlusTanh := tanhResult.AddScalar(1.0)

		// 0.5 * x * (1 + tanh(...))
		return x.MulScalar(0.5).Mul(onePlusTanh)
	}

	panic("GELUFunc: backend must implement Tanh operation (use autodiff.AutodiffBackend)")
}

// ReLUFunc applies ReLU activation: f(x) = max(0, x).
//
// Example:
//
//	output := nn.ReLUFunc(input)
func ReLUFunc[B tensor.Backend](x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	backend := x.Backend()

	// Check if backend supports ReLU via interface
	if reluBackend, ok := any(backend).(ReLUBackend); ok {
		resultRaw := reluBackend.ReLU(x.Raw())
		return tensor.New[float32, B](resultRaw, backend)
	}

	panic("ReLUFunc: backend must implement ReLU operation (use autodiff.AutodiffBackend)")
}

// SigmoidFunc applies Sigmoid activation: σ(x) = 1 / (1 + exp(-x)).
//
// Example:
//
//	output := nn.SigmoidFunc(input)
func SigmoidFunc[B tensor.Backend](x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	backend := x.Backend()

	// Check if backend supports Sigmoid via interface
	if sigmoidBackend, ok := any(backend).(SigmoidBackend); ok {
		resultRaw := sigmoidBackend.Sigmoid(x.Raw())
		return tensor.New[float32, B](resultRaw, backend)
	}

	panic("SigmoidFunc: backend must implement Sigmoid operation (use autodiff.AutodiffBackend)")
}

// GLU applies Gated Linear Unit: GLU(x, gate) = x * sigmoid(gate).
//
// GLU is the base gating mechanism used in various transformer FFN layers.
//
// Parameters:
//   - x: input tensor.
//   - gate: gating tensor (same shape as x).
//
// Returns: x * sigmoid(gate).
//
// Example:
//
//	output := nn.GLU(x, gate)
func GLU[B tensor.Backend](x, gate *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	return x.Mul(SigmoidFunc(gate))
}

// SwiGLU applies Swish-Gated Linear Unit: SwiGLU(x, gate) = x * SiLU(gate).
//
// SwiGLU is used in modern LLMs like LLaMA, Mistral, and DeepSeek.
// It combines the input with SiLU-activated gate for better gradient flow.
//
// Parameters:
//   - x: input tensor (typically "up" projection).
//   - gate: gating tensor (typically "gate" projection).
//
// Returns: x * SiLU(gate) where SiLU(z) = z * sigmoid(z).
//
// Example:
//
//	// In LLaMA-style FFN:
//	up := upProj.Forward(input)
//	gate := gateProj.Forward(input)
//	hidden := nn.SwiGLU(up, gate)
func SwiGLU[B tensor.Backend](x, gate *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	return x.Mul(SiLUFunc(gate))
}

// GeGLU applies GELU-Gated Linear Unit: GeGLU(x, gate) = x * GELU(gate).
//
// GeGLU uses GELU activation for gating instead of SiLU.
// Used in some transformer variants for different activation characteristics.
//
// Parameters:
//   - x: input tensor.
//   - gate: gating tensor.
//
// Returns: x * GELU(gate).
//
// Example:
//
//	output := nn.GeGLU(up, gate)
func GeGLU[B tensor.Backend](x, gate *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	return x.Mul(GELUFunc(gate))
}

// ReGLU applies ReLU-Gated Linear Unit: ReGLU(x, gate) = x * ReLU(gate).
//
// ReGLU uses ReLU activation for gating. It's simpler but may have
// "dead neuron" issues compared to SwiGLU or GeGLU.
//
// Parameters:
//   - x: input tensor.
//   - gate: gating tensor.
//
// Returns: x * ReLU(gate).
//
// Example:
//
//	output := nn.ReGLU(up, gate)
func ReGLU[B tensor.Backend](x, gate *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	return x.Mul(ReLUFunc(gate))
}
