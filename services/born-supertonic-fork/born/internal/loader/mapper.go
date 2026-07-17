package loader

import (
	"fmt"
	"strings"
)

// Architecture names.
const (
	ArchitectureLLaMA    = "llama"
	ArchitectureMistral  = "mistral"
	ArchitectureDeepSeek = "deepseek"
)

// Standard Born weight names used across all model architectures.
const (
	weightEmbedding = "embedding.weight"
	weightNorm      = "norm.weight"
	weightLMHead    = "lm_head.weight"
)

// GGML tensor names for the standard weights.
const (
	ggmlTokenEmbed = "token_embd.weight" //nolint:gosec // G101 false positive: this is a tensor name, not a credential
	ggmlOutput     = "output.weight"
	ggmlOutputNorm = "output_norm.weight"
)

// WeightMapper maps model-specific weight names to standard Born names.
type WeightMapper interface {
	// MapName converts a model-specific weight name to Born standard name.
	MapName(name string) (string, error)

	// Architecture returns the architecture name (e.g., "llama", "mistral").
	Architecture() string
}

// LLaMAMapper maps LLaMA weight names to Born standard names.
// Supports LLaMA, LLaMA 2, and LLaMA 3.
type LLaMAMapper struct{}

// NewLLaMAMapper creates a new LLaMA weight mapper.
func NewLLaMAMapper() *LLaMAMapper {
	return &LLaMAMapper{}
}

// MapName converts LLaMA weight names to Born standard names.
// LLaMA format:
//   - model.embed_tokens.weight -> embedding.weight
//   - model.layers.{i}.self_attn.q_proj.weight -> layers.{i}.attn.q.weight
//   - model.layers.{i}.mlp.gate_proj.weight -> layers.{i}.ffn.gate.weight
//   - model.norm.weight -> norm.weight
func (m *LLaMAMapper) MapName(name string) (string, error) {
	// Embedding
	if strings.HasPrefix(name, "model.embed_tokens.weight") {
		return weightEmbedding, nil
	}

	// Final norm
	if strings.HasPrefix(name, "model.norm.weight") {
		return weightNorm, nil
	}

	// Output/LM head
	if strings.HasPrefix(name, "lm_head.weight") {
		return weightLMHead, nil
	}

	// Layer-specific weights
	if strings.HasPrefix(name, "model.layers.") {
		return m.mapLayerWeight(name)
	}

	return name, nil // Return original if no mapping found
}

// mapLayerWeight maps LLaMA layer-specific weights.
func (m *LLaMAMapper) mapLayerWeight(name string) (string, error) {
	parts := strings.Split(name, ".")

	if len(parts) < 4 {
		return name, nil
	}

	// Extract layer index
	layerIdx := parts[2]

	// Attention weights
	if parts[3] == "self_attn" {
		switch {
		case strings.Contains(name, "q_proj.weight"):
			return fmt.Sprintf("layers.%s.attn.q.weight", layerIdx), nil
		case strings.Contains(name, "k_proj.weight"):
			return fmt.Sprintf("layers.%s.attn.k.weight", layerIdx), nil
		case strings.Contains(name, "v_proj.weight"):
			return fmt.Sprintf("layers.%s.attn.v.weight", layerIdx), nil
		case strings.Contains(name, "o_proj.weight"):
			return fmt.Sprintf("layers.%s.attn.o.weight", layerIdx), nil
		}
	}

	// MLP/FFN weights
	if parts[3] == "mlp" {
		switch {
		case strings.Contains(name, "gate_proj.weight"):
			return fmt.Sprintf("layers.%s.ffn.gate.weight", layerIdx), nil
		case strings.Contains(name, "up_proj.weight"):
			return fmt.Sprintf("layers.%s.ffn.up.weight", layerIdx), nil
		case strings.Contains(name, "down_proj.weight"):
			return fmt.Sprintf("layers.%s.ffn.down.weight", layerIdx), nil
		}
	}

	// Normalization weights
	switch {
	case strings.Contains(name, "input_layernorm.weight"):
		return fmt.Sprintf("layers.%s.norm1.weight", layerIdx), nil
	case strings.Contains(name, "post_attention_layernorm.weight"):
		return fmt.Sprintf("layers.%s.norm2.weight", layerIdx), nil
	}

	return name, nil
}

// Architecture returns "llama".
func (m *LLaMAMapper) Architecture() string {
	return ArchitectureLLaMA
}

// MistralMapper maps Mistral weight names to Born standard names.
// Supports Mistral 7B and Mixtral 8x7B.
type MistralMapper struct{}

// NewMistralMapper creates a new Mistral weight mapper.
func NewMistralMapper() *MistralMapper {
	return &MistralMapper{}
}

// MapName converts Mistral weight names to Born standard names.
// Mistral uses similar naming to LLaMA but with some differences.
func (m *MistralMapper) MapName(name string) (string, error) {
	// Mistral uses the same structure as LLaMA for most weights
	llamaMapper := NewLLaMAMapper()
	mapped, err := llamaMapper.MapName(name)
	if err != nil {
		return "", err
	}

	// Handle MoE-specific weights for Mixtral
	if strings.Contains(name, "block_sparse_moe") {
		return m.mapMoEWeight(name)
	}

	return mapped, nil
}

// mapMoEWeight maps Mixtral MoE (Mixture of Experts) weights.
func (m *MistralMapper) mapMoEWeight(name string) (string, error) {
	parts := strings.Split(name, ".")

	if len(parts) < 5 {
		return name, nil
	}

	layerIdx := parts[2]

	// MoE experts: model.layers.{i}.block_sparse_moe.experts.{j}.w1.weight
	if parts[4] == "experts" && len(parts) >= 7 {
		expertIdx := parts[5]
		switch parts[6] {
		case "w1.weight":
			return fmt.Sprintf("layers.%s.moe.experts.%s.w1.weight", layerIdx, expertIdx), nil
		case "w2.weight":
			return fmt.Sprintf("layers.%s.moe.experts.%s.w2.weight", layerIdx, expertIdx), nil
		case "w3.weight":
			return fmt.Sprintf("layers.%s.moe.experts.%s.w3.weight", layerIdx, expertIdx), nil
		}
	}

	// MoE gate: model.layers.{i}.block_sparse_moe.gate.weight
	if strings.Contains(name, "gate.weight") {
		return fmt.Sprintf("layers.%s.moe.gate.weight", layerIdx), nil
	}

	return name, nil
}

// Architecture returns "mistral".
func (m *MistralMapper) Architecture() string {
	return ArchitectureMistral
}

// DeepSeekMapper maps DeepSeek weight names to Born standard names.
// Supports DeepSeek-V2 and DeepSeek-Coder.
type DeepSeekMapper struct{}

// NewDeepSeekMapper creates a new DeepSeek weight mapper.
func NewDeepSeekMapper() *DeepSeekMapper {
	return &DeepSeekMapper{}
}

// MapName converts DeepSeek weight names to Born standard names.
// DeepSeek uses similar naming to LLaMA but may have architecture-specific differences.
func (m *DeepSeekMapper) MapName(name string) (string, error) {
	// DeepSeek-V2 uses Multi-head Latent Attention (MLA)
	if strings.Contains(name, "kv_a_proj") || strings.Contains(name, "kv_b_proj") {
		return m.mapMLAWeight(name)
	}

	// For other weights, use LLaMA mapping as base
	llamaMapper := NewLLaMAMapper()
	return llamaMapper.MapName(name)
}

// mapMLAWeight maps DeepSeek-V2 Multi-head Latent Attention weights.
func (m *DeepSeekMapper) mapMLAWeight(name string) (string, error) {
	parts := strings.Split(name, ".")

	if len(parts) < 5 {
		return name, nil
	}

	layerIdx := parts[2]

	switch {
	case strings.Contains(name, "kv_a_proj.weight"):
		return fmt.Sprintf("layers.%s.attn.kv_a.weight", layerIdx), nil
	case strings.Contains(name, "kv_b_proj.weight"):
		return fmt.Sprintf("layers.%s.attn.kv_b.weight", layerIdx), nil
	}

	return name, nil
}

// Architecture returns "deepseek".
func (m *DeepSeekMapper) Architecture() string {
	return ArchitectureDeepSeek
}

// GGMLMapper maps GGUF-native (ggml) weight names to Born standard names.
// GGUF files from llama.cpp use this naming convention (blk.0.attn_q.weight).
type GGMLMapper struct{}

// NewGGMLMapper creates a new GGML weight mapper.
func NewGGMLMapper() *GGMLMapper {
	return &GGMLMapper{}
}

// MapName converts GGUF-native tensor names to Born standard names.
func (m *GGMLMapper) MapName(name string) (string, error) {
	switch {
	case name == ggmlTokenEmbed:
		return weightEmbedding, nil
	case name == ggmlOutput:
		return weightLMHead, nil
	case name == ggmlOutputNorm:
		return weightNorm, nil
	case strings.HasPrefix(name, "blk."):
		return m.mapBlockWeight(name)
	}
	return name, nil
}

func (m *GGMLMapper) mapBlockWeight(name string) (string, error) {
	// blk.{i}.attn_q.weight -> layers.{i}.attn.q.weight
	parts := strings.Split(name, ".")
	if len(parts) < 3 {
		return name, nil
	}
	layerIdx := parts[1]

	rest := strings.Join(parts[2:], ".")
	switch rest {
	case "attn_norm.weight":
		return fmt.Sprintf("layers.%s.norm1.weight", layerIdx), nil
	case "ffn_norm.weight":
		return fmt.Sprintf("layers.%s.norm2.weight", layerIdx), nil
	case "attn_q.weight":
		return fmt.Sprintf("layers.%s.attn.q.weight", layerIdx), nil
	case "attn_k.weight":
		return fmt.Sprintf("layers.%s.attn.k.weight", layerIdx), nil
	case "attn_v.weight":
		return fmt.Sprintf("layers.%s.attn.v.weight", layerIdx), nil
	case "attn_output.weight":
		return fmt.Sprintf("layers.%s.attn.o.weight", layerIdx), nil
	case "ffn_gate.weight":
		return fmt.Sprintf("layers.%s.ffn.gate.weight", layerIdx), nil
	case "ffn_up.weight":
		return fmt.Sprintf("layers.%s.ffn.up.weight", layerIdx), nil
	case "ffn_down.weight":
		return fmt.Sprintf("layers.%s.ffn.down.weight", layerIdx), nil
	}
	return name, nil
}

// Architecture returns "llama".
func (m *GGMLMapper) Architecture() string {
	return ArchitectureLLaMA
}

// NamingFormat identifies the weight naming convention.
const (
	NamingHuggingFace = "hf"   // model.layers.0.self_attn.q_proj.weight
	NamingGGML        = "ggml" // blk.0.attn_q.weight
)

// DetectNaming identifies whether tensor names use HuggingFace or GGML convention.
func DetectNaming(names []string) string {
	for _, name := range names {
		if strings.HasPrefix(name, "blk.") || name == ggmlTokenEmbed {
			return NamingGGML
		}
		if strings.HasPrefix(name, "model.") {
			return NamingHuggingFace
		}
	}
	return NamingHuggingFace // default
}

// DetectArchitecture attempts to detect model architecture from weight names.
func DetectArchitecture(names []string) string {
	// Check for DeepSeek-specific weights
	for _, name := range names {
		if strings.Contains(name, "kv_a_proj") || strings.Contains(name, "kv_b_proj") {
			return ArchitectureDeepSeek
		}
	}

	// Check for Mixtral MoE weights
	for _, name := range names {
		if strings.Contains(name, "block_sparse_moe") {
			return ArchitectureMistral
		}
	}

	// Default to LLaMA (most common)
	return ArchitectureLLaMA
}

// GetMapperForNaming returns a mapper based on the naming convention of the GGUF file.
func GetMapperForNaming(names []string) WeightMapper {
	if DetectNaming(names) == NamingGGML {
		return NewGGMLMapper()
	}
	return GetMapper(DetectArchitecture(names))
}

// GetMapper returns the appropriate weight mapper for an architecture.
func GetMapper(architecture string) WeightMapper {
	switch architecture {
	case ArchitectureLLaMA:
		return NewLLaMAMapper()
	case ArchitectureMistral:
		return NewMistralMapper()
	case ArchitectureDeepSeek:
		return NewDeepSeekMapper()
	default:
		return NewLLaMAMapper() // Default to LLaMA
	}
}
