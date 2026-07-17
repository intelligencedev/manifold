package loader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGGMLMapper_MapName(t *testing.T) {
	m := NewGGMLMapper()

	tests := []struct {
		ggml string
		born string
	}{
		// Global tensors.
		{"token_embd.weight", "embedding.weight"},
		{"output.weight", "lm_head.weight"},
		{"output_norm.weight", "norm.weight"},

		// Layer 0 attention.
		{"blk.0.attn_q.weight", "layers.0.attn.q.weight"},
		{"blk.0.attn_k.weight", "layers.0.attn.k.weight"},
		{"blk.0.attn_v.weight", "layers.0.attn.v.weight"},
		{"blk.0.attn_output.weight", "layers.0.attn.o.weight"},

		// Layer 0 norms.
		{"blk.0.attn_norm.weight", "layers.0.norm1.weight"},
		{"blk.0.ffn_norm.weight", "layers.0.norm2.weight"},

		// Layer 0 FFN.
		{"blk.0.ffn_gate.weight", "layers.0.ffn.gate.weight"},
		{"blk.0.ffn_up.weight", "layers.0.ffn.up.weight"},
		{"blk.0.ffn_down.weight", "layers.0.ffn.down.weight"},

		// Layer 21 (TinyLlama last layer).
		{"blk.21.attn_q.weight", "layers.21.attn.q.weight"},
		{"blk.21.ffn_norm.weight", "layers.21.norm2.weight"},
	}

	for _, tt := range tests {
		t.Run(tt.ggml, func(t *testing.T) {
			got, err := m.MapName(tt.ggml)
			assert.NoError(t, err)
			assert.Equal(t, tt.born, got, "MapName(%q)", tt.ggml)
		})
	}
}

func TestGGMLMapper_Architecture(t *testing.T) {
	assert.Equal(t, ArchitectureLLaMA, NewGGMLMapper().Architecture())
}

func TestDetectNaming(t *testing.T) {
	tests := []struct {
		name   string
		names  []string
		expect string
	}{
		{"ggml", []string{"token_embd.weight", "blk.0.attn_q.weight"}, NamingGGML},
		{"hf", []string{"model.embed_tokens.weight", "model.layers.0.self_attn.q_proj.weight"}, NamingHuggingFace},
		{"empty", []string{}, NamingHuggingFace},
		{"mixed_ggml_first", []string{"blk.0.attn_q.weight", "model.layers.0.self_attn.q_proj.weight"}, NamingGGML},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, DetectNaming(tt.names))
		})
	}
}

func TestGetMapperForNaming(t *testing.T) {
	// GGML names → GGMLMapper.
	ggmlNames := []string{"token_embd.weight", "blk.0.attn_q.weight"}
	ggmlMapper := GetMapperForNaming(ggmlNames)
	got, _ := ggmlMapper.MapName("blk.0.attn_q.weight")
	assert.Equal(t, "layers.0.attn.q.weight", got)

	// HF names → LLaMAMapper.
	hfNames := []string{"model.embed_tokens.weight"}
	hfMapper := GetMapperForNaming(hfNames)
	got, _ = hfMapper.MapName("model.embed_tokens.weight")
	assert.Equal(t, "embedding.weight", got)
}

// TestGGMLMapper_AllTinyLlamaNames verifies mapping for every tensor in TinyLlama GGUF.
func TestGGMLMapper_AllTinyLlamaNames(t *testing.T) {
	m := NewGGMLMapper()

	// All 201 tensor names from TinyLlama 1.1B GGUF.
	globalNames := []string{"token_embd.weight", "output.weight", "output_norm.weight"}
	for _, name := range globalNames {
		got, err := m.MapName(name)
		assert.NoError(t, err, name)
		assert.NotEqual(t, name, got, "global tensor %q should be mapped", name)
	}

	// Per-layer names (22 layers).
	layerParts := []string{
		"attn_norm.weight", "ffn_norm.weight",
		"attn_q.weight", "attn_k.weight", "attn_v.weight", "attn_output.weight",
		"ffn_gate.weight", "ffn_up.weight", "ffn_down.weight",
	}
	for i := 0; i < 22; i++ {
		for _, part := range layerParts {
			name := "blk." + itoa(i) + "." + part
			got, err := m.MapName(name)
			assert.NoError(t, err, name)
			assert.NotEqual(t, name, got, "layer tensor %q should be mapped", name)
		}
	}
}

func itoa(i int) string {
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}
