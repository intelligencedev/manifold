package nn

import (
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// SwiGLUFFNConfig configures a SwiGLUFFN layer.
type SwiGLUFFNConfig = nn.SwiGLUFFNConfig

// SwiGLUFFN implements a feed-forward network with SwiGLU activation.
//
// Architecture (LLaMA-style):
//
//	hidden = SwiGLU(x @ W_up, x @ W_gate)
//	output = hidden @ W_down
//
// Where SwiGLU(up, gate) = up * SiLU(gate).
//
// This is more parameter-efficient than standard FFN with GELU.
// LLaMA uses ffn_dim = 2.7 * d_model (vs 4 * d_model in standard FFN)
// resulting in similar total parameters but better performance.
//
// Example:
//
//	backend := autodiff.New(cpu.New())
//	cfg := nn.SwiGLUFFNConfig{
//	    EmbedDim: 4096,
//	    FFNDim:   11008,  // LLaMA 7B
//	}
//	ffn := nn.NewSwiGLUFFN(cfg, backend)
//	output := ffn.Forward(x)  // [batch, seq, 4096] -> [batch, seq, 4096]
type SwiGLUFFN[B tensor.Backend] = nn.SwiGLUFFN[B]

// NewSwiGLUFFN creates a new SwiGLUFFN layer.
//
// If GLUVariant is empty, defaults to "swiglu".
// If FFNDim is 0, it's computed as 8/3 * EmbedDim (LLaMA formula).
//
// Example:
//
//	// LLaMA 7B FFN
//	ffn := nn.NewSwiGLUFFN(nn.SwiGLUFFNConfig{
//	    EmbedDim: 4096,
//	    FFNDim:   11008,
//	}, backend)
func NewSwiGLUFFN[B tensor.Backend](cfg SwiGLUFFNConfig, backend B) *SwiGLUFFN[B] {
	return nn.NewSwiGLUFFN(cfg, backend)
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
	return nn.SwiGLU(x, gate)
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
	return nn.GeGLU(x, gate)
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
	return nn.ReGLU(x, gate)
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
	return nn.GLU(x, gate)
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
	return nn.GELUFunc(x)
}

// SiLUFunc applies SiLU (Swish) activation: f(x) = x * sigmoid(x).
//
// This is the functional version of SiLU activation, useful in GLU variants.
//
// Example:
//
//	output := nn.SiLUFunc(input)
func SiLUFunc[B tensor.Backend](x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	return nn.SiLUFunc(x)
}
