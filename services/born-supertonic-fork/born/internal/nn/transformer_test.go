package nn

import (
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// TestFFN_Forward tests FFN forward pass with 2D input.
func TestFFN_Forward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// GPT-2 small: 768 -> 3072 -> 768
	ffn := NewFFN[*autodiff.AutodiffBackend[*cpu.CPUBackend]](768, 3072, backend)

	// Input: [batch=2, embed_dim=768]
	x := tensor.Randn[float32](tensor.Shape{2, 768}, backend)

	// Forward
	output := ffn.Forward(x)

	// Check shape
	if !shapeEqual(output.Shape(), tensor.Shape{2, 768}) {
		t.Errorf("Expected shape [2, 768], got %v", output.Shape())
	}

	// Check parameters count
	params := ffn.Parameters()
	totalParams := 0
	for _, p := range params {
		totalParams += p.Tensor().NumElements()
	}

	// Linear1: 768*3072 weights + 3072 biases = 2,362,368
	// Linear2: 3072*768 weights + 768 biases = 2,360,064
	// Total: 4,722,432
	expected := 768*3072 + 3072 + 3072*768 + 768
	if totalParams != expected {
		t.Errorf("Expected %d parameters, got %d", expected, totalParams)
	}
}

// TestFFN_Forward3D tests FFN forward pass with 3D input (batch, seq, embed_dim).
func TestFFN_Forward3D(t *testing.T) {
	backend := autodiff.New(cpu.New())

	ffn := NewFFN[*autodiff.AutodiffBackend[*cpu.CPUBackend]](512, 2048, backend)

	// Input: [batch=2, seq=10, embed_dim=512]
	x := tensor.Randn[float32](tensor.Shape{2, 10, 512}, backend)

	// Forward
	output := ffn.Forward(x)

	// Check shape (should preserve 3D shape)
	if !shapeEqual(output.Shape(), tensor.Shape{2, 10, 512}) {
		t.Errorf("Expected shape [2, 10, 512], got %v", output.Shape())
	}
}

// TestTransformerBlock_PreNorm tests Pre-Norm architecture.
func TestTransformerBlock_PreNorm(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   768,
		NumHeads:   12,
		FFNDim:     3072,
		Dropout:    0,
		NormFirst:  true, // Pre-Norm
		UseRMSNorm: true, // RMSNorm
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)

	// Input: [batch=2, seq=16, embed_dim=768]
	x := tensor.Randn[float32](tensor.Shape{2, 16, 768}, backend)

	// Forward
	output := block.Forward(x, nil)

	// Check shape
	if !shapeEqual(output.Shape(), tensor.Shape{2, 16, 768}) {
		t.Errorf("Expected shape [2, 16, 768], got %v", output.Shape())
	}
}

// TestTransformerBlock_PostNorm tests Post-Norm architecture.
func TestTransformerBlock_PostNorm(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   768,
		NumHeads:   12,
		FFNDim:     3072,
		Dropout:    0,
		NormFirst:  false, // Post-Norm
		UseRMSNorm: false, // LayerNorm
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)

	// Input: [batch=2, seq=16, embed_dim=768]
	x := tensor.Randn[float32](tensor.Shape{2, 16, 768}, backend)

	// Forward
	output := block.Forward(x, nil)

	// Check shape
	if !shapeEqual(output.Shape(), tensor.Shape{2, 16, 768}) {
		t.Errorf("Expected shape [2, 16, 768], got %v", output.Shape())
	}
}

// TestTransformerBlock_WithRMSNorm tests transformer block with RMSNorm.
func TestTransformerBlock_WithRMSNorm(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   512,
		NumHeads:   8,
		FFNDim:     2048,
		NormFirst:  true,
		UseRMSNorm: true, // RMSNorm
		NormEps:    1e-6,
	}
	block := NewTransformerBlock(config, backend)

	x := tensor.Randn[float32](tensor.Shape{1, 10, 512}, backend)
	output := block.Forward(x, nil)

	if !shapeEqual(output.Shape(), tensor.Shape{1, 10, 512}) {
		t.Errorf("Expected shape [1, 10, 512], got %v", output.Shape())
	}
}

// TestTransformerBlock_WithLayerNorm tests transformer block with LayerNorm.
func TestTransformerBlock_WithLayerNorm(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   512,
		NumHeads:   8,
		FFNDim:     2048,
		NormFirst:  true,
		UseRMSNorm: false, // LayerNorm
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)

	x := tensor.Randn[float32](tensor.Shape{1, 10, 512}, backend)
	output := block.Forward(x, nil)

	if !shapeEqual(output.Shape(), tensor.Shape{1, 10, 512}) {
		t.Errorf("Expected shape [1, 10, 512], got %v", output.Shape())
	}
}

// TestTransformerBlock_ParameterCount tests parameter count (GPT-2 small).
func TestTransformerBlock_ParameterCount(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   768,
		NumHeads:   12,
		FFNDim:     3072,
		NormFirst:  true,
		UseRMSNorm: true, // RMSNorm (fewer params than LayerNorm)
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)

	params := block.Parameters()

	// Count total parameters
	totalParams := 0
	for _, p := range params {
		totalParams += p.Tensor().NumElements()
	}

	// Expected (GPT-2 768d/12h with RMSNorm):
	// AttnNorm: 768 (RMSNorm = just gamma)
	// MHA:
	//   WQ: 768*768 + 768 = 590,592
	//   WK: 768*768 + 768 = 590,592
	//   WV: 768*768 + 768 = 590,592
	//   WO: 768*768 + 768 = 590,592
	//   Total MHA: 2,362,368
	// FFNNorm: 768 (RMSNorm)
	// FFN:
	//   Linear1: 768*3072 + 3072 = 2,362,368
	//   Linear2: 3072*768 + 768 = 2,360,064
	//   Total FFN: 4,722,432
	// Grand Total: 768 + 2,362,368 + 768 + 4,722,432 = 7,086,336

	expected := 7_086_336
	if totalParams != expected {
		t.Errorf("Expected %d parameters, got %d", expected, totalParams)
	}

	// Should be ~7.1M parameters per block
	if totalParams < 7_000_000 || totalParams > 7_100_000 {
		t.Errorf("Expected ~7.1M parameters, got %d", totalParams)
	}
}

// TestTransformerBlock_ParameterCount_LayerNorm tests parameter count with LayerNorm.
func TestTransformerBlock_ParameterCount_LayerNorm(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   768,
		NumHeads:   12,
		FFNDim:     3072,
		NormFirst:  true,
		UseRMSNorm: false, // LayerNorm (more params: gamma + beta)
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)

	params := block.Parameters()
	totalParams := 0
	for _, p := range params {
		totalParams += p.Tensor().NumElements()
	}

	// With LayerNorm:
	// AttnNorm: 768*2 = 1536 (gamma + beta)
	// MHA: 2,362,368
	// FFNNorm: 768*2 = 1536
	// FFN: 4,722,432
	// Total: 1536 + 2,362,368 + 1536 + 4,722,432 = 7,087,872

	expected := 7_087_872
	if totalParams != expected {
		t.Errorf("Expected %d parameters, got %d", expected, totalParams)
	}
}

// TestTransformerBlock_Stack tests stacking multiple transformer blocks.
func TestTransformerBlock_Stack(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   512,
		NumHeads:   8,
		FFNDim:     2048,
		NormFirst:  true,
		UseRMSNorm: true,
		NormEps:    1e-5,
	}

	// Create 3 blocks (like mini-GPT)
	blocks := make([]*TransformerBlock[*autodiff.AutodiffBackend[*cpu.CPUBackend]], 3)
	for i := range blocks {
		blocks[i] = NewTransformerBlock(config, backend)
	}

	// Input: [batch=1, seq=8, embed_dim=512]
	x := tensor.Randn[float32](tensor.Shape{1, 8, 512}, backend)

	// Forward through all blocks
	for _, block := range blocks {
		x = block.Forward(x, nil)
	}

	// Final shape should be same as input
	if !shapeEqual(x.Shape(), tensor.Shape{1, 8, 512}) {
		t.Errorf("Expected shape [1, 8, 512], got %v", x.Shape())
	}
}

// TestTransformerBlock_ForwardWithCache tests cache-based generation.
func TestTransformerBlock_ForwardWithCache(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   512,
		NumHeads:   8,
		FFNDim:     2048,
		NormFirst:  true, // Required for cache
		UseRMSNorm: true,
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)

	// Create cache: batch=1, heads=8, maxSeq=100, headDim=64
	cache := NewKVCache[*autodiff.AutodiffBackend[*cpu.CPUBackend]](1, 8, 100, 64, backend)

	// Generate 10 tokens one by one
	for i := 0; i < 10; i++ {
		// Input: [batch=1, seq=1, embed_dim=512] (single token)
		token := tensor.Randn[float32](tensor.Shape{1, 1, 512}, backend)

		// Forward with cache
		output := block.ForwardWithCache(token, cache)

		// Check shape
		if !shapeEqual(output.Shape(), tensor.Shape{1, 1, 512}) {
			t.Errorf("Token %d: expected shape [1, 1, 512], got %v", i, output.Shape())
		}

		// Check cache length
		if cache.Len() != i+1 {
			t.Errorf("Token %d: expected cache length %d, got %d", i, i+1, cache.Len())
		}
	}

	// Final cache should have 10 tokens
	if cache.Len() != 10 {
		t.Errorf("Expected cache length 10, got %d", cache.Len())
	}
}

// TestTransformerBlock_ForwardWithCache_PostNormPanic tests that Post-Norm panics with cache.
func TestTransformerBlock_ForwardWithCache_PostNormPanic(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   512,
		NumHeads:   8,
		FFNDim:     2048,
		NormFirst:  false, // Post-Norm
		UseRMSNorm: true,
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)

	cache := NewKVCache[*autodiff.AutodiffBackend[*cpu.CPUBackend]](1, 8, 100, 64, backend)
	token := tensor.Randn[float32](tensor.Shape{1, 1, 512}, backend)

	// Should panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for Post-Norm with cache, but didn't panic")
		}
	}()

	block.ForwardWithCache(token, cache)
}

// TestTransformerBlock_InvalidConfig tests configuration validation.
func TestTransformerBlock_InvalidConfig(t *testing.T) {
	backend := autodiff.New(cpu.New())

	tests := []struct {
		name   string
		config TransformerConfig
	}{
		{
			name: "embedDim not divisible by numHeads",
			config: TransformerConfig{
				EmbedDim:   765, // Not divisible by 12
				NumHeads:   12,
				FFNDim:     3072,
				NormFirst:  true,
				UseRMSNorm: true,
				NormEps:    1e-5,
			},
		},
		{
			name: "negative embedDim",
			config: TransformerConfig{
				EmbedDim:   -768,
				NumHeads:   12,
				FFNDim:     3072,
				NormFirst:  true,
				UseRMSNorm: true,
				NormEps:    1e-5,
			},
		},
		{
			name: "negative ffnDim",
			config: TransformerConfig{
				EmbedDim:   768,
				NumHeads:   12,
				FFNDim:     -3072,
				NormFirst:  true,
				UseRMSNorm: true,
				NormEps:    1e-5,
			},
		},
		{
			name: "negative normEps",
			config: TransformerConfig{
				EmbedDim:   768,
				NumHeads:   12,
				FFNDim:     3072,
				NormFirst:  true,
				UseRMSNorm: true,
				NormEps:    -1e-5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Expected panic for invalid config, but didn't panic")
				}
			}()

			NewTransformerBlock(tt.config, backend)
		})
	}
}

// TestTransformerBlock_WithMask tests attention mask.
func TestTransformerBlock_WithMask(t *testing.T) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   256,
		NumHeads:   4,
		FFNDim:     1024,
		NormFirst:  true,
		UseRMSNorm: true,
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)

	// Input: [batch=1, seq=8, embed_dim=256]
	x := tensor.Randn[float32](tensor.Shape{1, 8, 256}, backend)

	// Create causal mask [1, 1, 8, 8]
	mask := CausalMask(8, backend)

	// Forward with mask
	output := block.Forward(x, mask)

	// Check shape
	if !shapeEqual(output.Shape(), tensor.Shape{1, 8, 256}) {
		t.Errorf("Expected shape [1, 8, 256], got %v", output.Shape())
	}
}

// Benchmarks

func BenchmarkFFN_Forward(b *testing.B) {
	backend := autodiff.New(cpu.New())
	ffn := NewFFN[*autodiff.AutodiffBackend[*cpu.CPUBackend]](768, 3072, backend)
	x := tensor.Randn[float32](tensor.Shape{16, 768}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ffn.Forward(x)
	}
}

func BenchmarkTransformerBlock_PreNorm(b *testing.B) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   768,
		NumHeads:   12,
		FFNDim:     3072,
		NormFirst:  true,
		UseRMSNorm: true,
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)
	x := tensor.Randn[float32](tensor.Shape{4, 32, 768}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block.Forward(x, nil)
	}
}

func BenchmarkTransformerBlock_PostNorm(b *testing.B) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   768,
		NumHeads:   12,
		FFNDim:     3072,
		NormFirst:  false,
		UseRMSNorm: true,
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)
	x := tensor.Randn[float32](tensor.Shape{4, 32, 768}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block.Forward(x, nil)
	}
}

func BenchmarkTransformerBlock_WithCache(b *testing.B) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   768,
		NumHeads:   12,
		FFNDim:     3072,
		NormFirst:  true,
		UseRMSNorm: true,
		NormEps:    1e-5,
	}
	block := NewTransformerBlock(config, backend)
	cache := NewKVCache[*autodiff.AutodiffBackend[*cpu.CPUBackend]](1, 12, 512, 64, backend)
	token := tensor.Randn[float32](tensor.Shape{1, 1, 768}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%512 == 0 {
			cache.Reset() // Reset when full
		}
		block.ForwardWithCache(token, cache)
	}
}

func BenchmarkTransformerBlock_Stack3(b *testing.B) {
	backend := autodiff.New(cpu.New())

	config := TransformerConfig{
		EmbedDim:   512,
		NumHeads:   8,
		FFNDim:     2048,
		NormFirst:  true,
		UseRMSNorm: true,
		NormEps:    1e-5,
	}

	blocks := make([]*TransformerBlock[*autodiff.AutodiffBackend[*cpu.CPUBackend]], 3)
	for i := range blocks {
		blocks[i] = NewTransformerBlock(config, backend)
	}

	x := tensor.Randn[float32](tensor.Shape{2, 16, 512}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := x
		for _, block := range blocks {
			out = block.Forward(out, nil)
		}
	}
}
