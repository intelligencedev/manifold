package nn

import (
	"github.com/born-ml/born/internal/tensor"
)

// FFN implements a Feed-Forward Network (also called MLP - Multi-Layer Perceptron).
//
// Architecture:
//
//	FFN(x) = Linear2(SiLU(Linear1(x)))
//
// Where:
//   - Linear1: [embed_dim → ffn_dim] (expansion)
//   - SiLU: Activation function (x * sigmoid(x))
//   - Linear2: [ffn_dim → embed_dim] (projection back)
//
// The FFN is a core component of transformer blocks, typically with ffn_dim = 4 * embed_dim.
// This expansion-and-projection pattern helps the model learn complex transformations.
//
// Used in all transformer architectures:
//   - GPT: embed_dim=768, ffn_dim=3072 (4x expansion)
//   - BERT: embed_dim=768, ffn_dim=3072
//   - LLaMA: embed_dim=4096, ffn_dim=11008 (~2.7x expansion)
//
// Example:
//
//	backend := autodiff.New(cpu.New())
//	ffn := nn.NewFFN[B](768, 3072, backend)  // GPT-2 small
//	output := ffn.Forward(x)  // [batch, seq, 768] -> [batch, seq, 768]
type FFN[B tensor.Backend] struct {
	Linear1 *Linear[B] // [embed_dim → ffn_dim]
	Linear2 *Linear[B] // [ffn_dim → embed_dim]
	SiLU    *SiLU[B]   // Activation function
	backend B
}

// NewFFN creates a new Feed-Forward Network.
//
// Parameters:
//   - embedDim: Input/output dimension (e.g., 768 for GPT-2)
//   - ffnDim: Hidden dimension (typically 4 * embedDim)
//   - backend: Computation backend
//
// The network expands the input from embedDim to ffnDim, applies SiLU activation,
// then projects back to embedDim.
//
// Example:
//
//	ffn := nn.NewFFN[B](768, 3072, backend)  // GPT-2 small
func NewFFN[B tensor.Backend](embedDim, ffnDim int, backend B) *FFN[B] {
	return &FFN[B]{
		Linear1: NewLinear[B](embedDim, ffnDim, backend),
		Linear2: NewLinear[B](ffnDim, embedDim, backend),
		SiLU:    NewSiLU[B](),
		backend: backend,
	}
}

// Forward computes the FFN output.
//
// Shapes:
//   - input: [batch, seq, embed_dim] (3D) or [batch, embed_dim] (2D)
//   - output: same shape as input
//
// Algorithm:
//  1. Expand: x -> Linear1(x) [embed_dim → ffn_dim]
//  2. Activate: x -> SiLU(x)
//  3. Project: x -> Linear2(x) [ffn_dim → embed_dim]
//
// Note: Linear layers expect 2D input [batch, features], so we reshape if needed.
func (f *FFN[B]) Forward(x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// Handle both 2D [batch, embed_dim] and 3D [batch, seq, embed_dim]
	origShape := x.Shape()
	is3D := len(origShape) == 3

	var batch, seq, embedDim int
	if is3D {
		batch = origShape[0]
		seq = origShape[1]
		embedDim = origShape[2]
		// Reshape to 2D for Linear layers: [batch, seq, embed_dim] -> [batch*seq, embed_dim]
		x = x.Reshape(batch*seq, embedDim)
	}

	// 1. Expand: Linear1 [embed_dim → ffn_dim]
	x = f.Linear1.Forward(x)

	// 2. Activate: SiLU(x) = x * sigmoid(x)
	x = f.SiLU.Forward(x)

	// 3. Project: Linear2 [ffn_dim → embed_dim]
	x = f.Linear2.Forward(x)

	// Reshape back to original shape if needed
	if is3D {
		x = x.Reshape(batch, seq, embedDim)
	}

	return x
}

// Parameters returns all trainable parameters (Linear1 and Linear2).
func (f *FFN[B]) Parameters() []*Parameter[B] {
	params := make([]*Parameter[B], 0, 4)
	params = append(params, f.Linear1.Parameters()...)
	params = append(params, f.Linear2.Parameters()...)
	return params
}
