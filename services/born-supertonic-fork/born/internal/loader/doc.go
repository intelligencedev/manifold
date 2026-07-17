// Package loader provides model weight loading functionality for Born ML framework.
//
// This package implements readers for popular model weight formats:
//   - SafeTensors: Zero-copy loading with memory mapping (Hugging Face standard)
//   - GGUF: Efficient format for quantized models (llama.cpp ecosystem)
//
// Supported architectures:
//   - LLaMA (including LLaMA 2, LLaMA 3)
//   - Mistral (7B, 8x7B MoE)
//   - DeepSeek (DeepSeek-V2, DeepSeek-Coder)
//
// The loader package focuses on inference use cases for v0.5.0 LLM support.
// Training is not supported in this version.
//
// Example:
//
//	// Auto-detect format and load model
//	model, err := loader.OpenModel("path/to/model.safetensors")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer model.Close()
//
//	// Load specific tensor
//	tensor, err := model.LoadTensor("model.layers.0.attn.q_proj.weight")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Design principles:
//   - Pure Go: No CGO dependencies
//   - Zero-copy: Memory mapping when possible (Unix/Darwin)
//   - Lazy loading: Load tensors on-demand
//   - Type safety: Proper dtype handling (F16, BF16, F32, Q4_0, Q8_0)
package loader
