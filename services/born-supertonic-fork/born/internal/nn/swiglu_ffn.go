package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

const (
	gluVariantSwiGLU = "swiglu"
	gluVariantGeGLU  = "geglu"
	gluVariantReGLU  = "reglu"
	gluVariantGLU    = "glu"
)

// SwiGLUFFNConfig configures a SwiGLUFFN layer.
type SwiGLUFFNConfig struct {
	EmbedDim   int    // Model dimension (d_model), e.g., 4096.
	FFNDim     int    // Intermediate/hidden dimension, e.g., 11008 for LLaMA 7B.
	GLUVariant string // Variant: "swiglu" (default), "geglu", "reglu", "glu".
	UseBias    bool   // Whether to use bias in linear layers (LLaMA doesn't).
}

// SwiGLUFFN implements a feed-forward network with SwiGLU activation.
//
// Architecture (LLaMA-style):
//
//	hidden = SwiGLU(x @ W_up, x @ W_gate)
//	output = hidden @ W_down
//
// Where SwiGLU(up, gate) = up * SiLU(gate).
//
// This is more parameter-efficient than standard FFN with GELU:
//   - Standard FFN: 2 * d_model * ffn_dim parameters.
//   - SwiGLU FFN: 3 * d_model * ffn_dim parameters (but ffn_dim is smaller).
//
// LLaMA uses ffn_dim = 2.7 * d_model (vs 4 * d_model in standard FFN)
// resulting in similar total parameters but better performance.
//
// Example:
//
//	cfg := nn.SwiGLUFFNConfig{
//	    EmbedDim: 4096,
//	    FFNDim:   11008,  // LLaMA 7B
//	}
//	ffn := nn.NewSwiGLUFFN(cfg, backend)
//	output := ffn.Forward(x)  // [batch, seq, 4096] -> [batch, seq, 4096]
type SwiGLUFFN[B tensor.Backend] struct {
	gateProj *Linear[B] // d_model -> ffn_dim (gate projection)
	upProj   *Linear[B] // d_model -> ffn_dim (up projection)
	downProj *Linear[B] // ffn_dim -> d_model (down projection)

	config  SwiGLUFFNConfig
	backend B
}

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
	if cfg.EmbedDim <= 0 {
		panic(fmt.Sprintf("SwiGLUFFN: EmbedDim must be positive, got %d", cfg.EmbedDim))
	}

	// Default FFN dimension (LLaMA formula: 8/3 * d_model, rounded to multiple of 256)
	if cfg.FFNDim <= 0 {
		cfg.FFNDim = (cfg.EmbedDim * 8 / 3)
		// Round to multiple of 256 for efficiency
		cfg.FFNDim = ((cfg.FFNDim + 255) / 256) * 256
	}

	// Default variant
	if cfg.GLUVariant == "" {
		cfg.GLUVariant = gluVariantSwiGLU
	}

	// Validate variant
	switch cfg.GLUVariant {
	case gluVariantSwiGLU, gluVariantGeGLU, gluVariantReGLU, gluVariantGLU:
		// Valid
	default:
		panic(fmt.Sprintf("SwiGLUFFN: unknown GLUVariant %q, expected swiglu/geglu/reglu/glu", cfg.GLUVariant))
	}

	// Create projections using WithBias option
	// Note: LLaMA doesn't use bias in FFN layers
	biasOpt := WithBias(cfg.UseBias)
	gateProj := NewLinear[B](cfg.EmbedDim, cfg.FFNDim, backend, biasOpt)
	upProj := NewLinear[B](cfg.EmbedDim, cfg.FFNDim, backend, biasOpt)
	downProj := NewLinear[B](cfg.FFNDim, cfg.EmbedDim, backend, biasOpt)

	return &SwiGLUFFN[B]{
		gateProj: gateProj,
		upProj:   upProj,
		downProj: downProj,
		config:   cfg,
		backend:  backend,
	}
}

// Forward computes the SwiGLU FFN output.
//
// Input: [batch, seq_len, embed_dim] or [batch*seq_len, embed_dim].
// Output: same shape as input.
//
// Computation:
//
//	gate = x @ W_gate
//	up = x @ W_up
//	hidden = GLU_variant(up, gate)  // e.g., up * SiLU(gate)
//	output = hidden @ W_down
func (f *SwiGLUFFN[B]) Forward(x *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	shape := x.Shape()
	is3D := len(shape) == 3

	var batch, seq, embedDim int
	if is3D {
		batch = shape[0]
		seq = shape[1]
		embedDim = shape[2]
		// Reshape to 2D for linear layers
		x = x.Reshape(batch*seq, embedDim)
	}

	// Compute gate and up projections
	gate := f.gateProj.Forward(x)
	up := f.upProj.Forward(x)

	// Apply GLU variant
	var hidden *tensor.Tensor[float32, B]
	switch f.config.GLUVariant {
	case gluVariantSwiGLU:
		hidden = SwiGLU(up, gate)
	case gluVariantGeGLU:
		hidden = GeGLU(up, gate)
	case gluVariantReGLU:
		hidden = ReGLU(up, gate)
	case gluVariantGLU:
		hidden = GLU(up, gate)
	default:
		hidden = SwiGLU(up, gate) // Default to SwiGLU
	}

	// Down projection
	output := f.downProj.Forward(hidden)

	// Reshape back to 3D if needed
	if is3D {
		output = output.Reshape(batch, seq, embedDim)
	}

	return output
}

// Parameters returns all trainable parameters.
func (f *SwiGLUFFN[B]) Parameters() []*Parameter[B] {
	params := make([]*Parameter[B], 0, 6)
	params = append(params, f.gateProj.Parameters()...)
	params = append(params, f.upProj.Parameters()...)
	params = append(params, f.downProj.Parameters()...)
	return params
}

// GateProj returns the gate projection layer.
func (f *SwiGLUFFN[B]) GateProj() *Linear[B] {
	return f.gateProj
}

// UpProj returns the up projection layer.
func (f *SwiGLUFFN[B]) UpProj() *Linear[B] {
	return f.upProj
}

// DownProj returns the down projection layer.
func (f *SwiGLUFFN[B]) DownProj() *Linear[B] {
	return f.downProj
}
